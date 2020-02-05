package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	speech "cloud.google.com/go/speech/apiv1p1beta1"
	redis "github.com/go-redis/redis/v7"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1p1beta1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

var (
	electionPort = flag.Int("electionPort", 4040,
		"Listen at this port for leader election updates. Set to zero to disable leader election")
	redisHost          = flag.String("redisHost", "localhost", "Redis host IP")
	audioQueue         = flag.String("audioQueue", "liveq", "Redis key for input audio data")
	transcriptionQueue = flag.String("transcriptionQueue", "transcriptions", "Redis key for output transcriptions")
	recoveryQueue      = flag.String("recoveryQueue", "recoverq", "Redis key for recent audio data used for recovery")
	recoveryRetainSecs = flag.Int("recoveryRetainLast", 4, "Retain the last N seconds of audio, replayed during recovery")
	recoveryTimeout    = flag.Duration("recoveryExpiry", 30, "Expire data in recovery queue after this time (seconds)")
	flushTimeout       = flag.Duration("flushTimeout", 2500, "Emit any pending transcriptions after this time (millis)")
	pendingWordCount   = flag.Int("pendingWordCount", 4, "Treat last N transcribed words as pending")
	sampleRate         = flag.Int("sampleRate", 16000, "Sample rate (Hz)")
	channels           = flag.Int("channels", 1, "Number of audio channels")
	lang               = flag.String("lang", "en-US", "Transcription language code")
	phrases            = flag.String("phrases", "", "Comma-separated list of phrase hints for Speech API. Phrases with spaces should be quoted")

	redisClient      *redis.Client
	recoveryExpiry   = *recoveryTimeout * time.Second
	recoveryRetain   = int64(*recoveryRetainSecs * 10) // each audio element ~100ms
	lastIndex        = 0
	latestTranscript string
	pending          []string
	unstable         string
)

func main() {
	flag.Parse()
	klog.InitFlags(nil)

	redisClient = redis.NewClient(&redis.Options{
		Addr:        *redisHost + ":6379",
		Password:    "", // no password set
		DB:          0,  // use default DB
		DialTimeout: 3 * time.Second,
		ReadTimeout: 4 * time.Second,
	})

	speechClient, err := speech.NewClient(context.Background())
	if err != nil {
		klog.Fatal(err)
	}

	contextPhrases := []string{}
	if *phrases != "" {
		contextPhrases = strings.Split(*phrases, ",")
		klog.Infof("Supplying %d phrase hints: %+q", len(contextPhrases), contextPhrases)
	}
	streamingConfig := speechpb.StreamingRecognitionConfig{
		Config: &speechpb.RecognitionConfig{
			Encoding:                   speechpb.RecognitionConfig_LINEAR16,
			SampleRateHertz:            int32(*sampleRate),
			AudioChannelCount:          int32(*channels),
			LanguageCode:               *lang,
			EnableAutomaticPunctuation: true,
			SpeechContexts: []*speechpb.SpeechContext{
				{Phrases: contextPhrases},
			},
		},
		InterimResults: true,
	}

	// if a port is defined, listen there for callbacks from leader election
	if *electionPort > 0 {
		var ctx context.Context
		var cancel context.CancelFunc
		addr := fmt.Sprintf(":%d", *electionPort)
		server := &http.Server{Addr: addr}

		// listen for interrupt. If so, cancel the context to gracefully stop transcriptions,
		// then shutdown the HTTP server, which will end the main thread.
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-ch
			if cancel != nil {
				cancel()
			}
			klog.Info("Received termination, stopping transciptions")
			if err := server.Shutdown(context.TODO()); err != nil {
				klog.Fatalf("Error shutting down HTTP server: %v", err)
			}
		}()

		// TODO: make atomic
		isLeader := false
		webHandler := func(res http.ResponseWriter, req *http.Request) {
			// if we're elected as leader, start a goroutine to read audio from
			// Redis and stream to Cloud Speech
			if strings.Contains(req.URL.Path, "start") {
				if !isLeader {
					isLeader = true
					klog.Infof("I became the leader! Starting goroutine to send audio")
					ctx, cancel = context.WithCancel(context.Background())
					go sendAudio(ctx, speechClient, streamingConfig)
				}
			}
			// if we stop being the leader, stop sending audio
			if strings.Contains(req.URL.Path, "stop") {
				if isLeader {
					isLeader = false
					klog.Infof("I stopped being the leader!")
					cancel()
				}
			}
			res.WriteHeader(http.StatusOK)
		}
		http.HandleFunc("/", webHandler)
		klog.Infof("Starting leader election listener at port %s", addr)
		// blocks
		server.ListenAndServe()
	} else {
		klog.Info("Not doing leader election")
		sendAudio(context.Background(), speechClient, streamingConfig)
	}
}

// Consumes queued audio data from Redis, and sends to Speech API.
func sendAudio(ctx context.Context, speechClient *speech.Client, config speechpb.StreamingRecognitionConfig) {
	var stream speechpb.Speech_StreamingRecognizeClient
	receiveChan := make(chan bool)
	doRecovery := redisClient.Exists(*recoveryQueue).Val() > 0
	// if doRecovery {
	// 	klog.Infof("Data in %s, sending that first", *recoveryQueue)
	// 	emit("[RECOVER]")
	// }
	// doRecovery := true
	recovering := false

	for {
		select {
		case <-ctx.Done():
			klog.Infof("Context cancelled, exiting sender loop")
			return
		case _, ok := <-receiveChan:
			if !ok && stream != nil {
				klog.Info("done channel closed, resetting stream")
				stream = nil
				receiveChan = make(chan bool)
				reset()
				continue
			}
		default:
			// process audio
		}

		var result string
		var err error
		if doRecovery {
			result, err = redisClient.RPop(*recoveryQueue).Result()
			if err == redis.Nil { // no more recovery data
				doRecovery = false
				recovering = false
				continue
			}
			if err == nil && !recovering {
				recovering = true
				emit("[RECOVER]")
			}
		} else {
			// Blocking pop audio data off the queue, and push onto recovery queue.
			// See https://redis.io/commands/rpoplpush#pattern-reliable-queue.
			// Timeout occasionally for hygiene.
			result, err = redisClient.BRPopLPush(*audioQueue, *recoveryQueue, 5*time.Second).Result()
			if err == redis.Nil { // pop timeout
				continue
			}
			// if err != nil {
			// 	if strings.HasPrefix(err.Error(), "READONLY ") {
			// 		klog.Infof("Redis failover? Attempting recovery: %v", err)
			// 		doRecovery = true
			// 	} else {
			// 		klog.Errorf("Could not read from Redis: %v", err)
			// 	}
			// 	continue
			// }
			if err == nil {
				// Retain only the last N seconds in the recovery queue
				redisClient.LTrim(*recoveryQueue, 0, recoveryRetain)
				// Expire the recovery data after M seconds
				redisClient.Expire(*recoveryQueue, recoveryExpiry)
			}
		}
		if err != nil && err != redis.Nil {
			klog.Errorf("Could not read from Redis: %v", err)
			// flush()
			doRecovery = true
			continue
		}

		// establish bi-directional connection to Speech API if necessary.
		// If so, start a go routine to listen for responses
		if stream == nil {
			stream = initStreamingRequest(ctx, speechClient, config)
			go receiveResponses(stream, receiveChan)
			emit("[NEW STREAM]")
		}

		// Send audio, transcription responses received asynchronously
		decoded, _ := base64.StdEncoding.DecodeString(result)
		sendErr := stream.Send(&speechpb.StreamingRecognizeRequest{
			StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
				AudioContent: decoded,
			},
		})
		if sendErr != nil {
			// expected - if stream has been closed (e.g. timeout)
			if sendErr == io.EOF {
				continue
			}
			klog.Errorf("Could not send audio: %v", sendErr)
		}
	}
}

func receiveResponses(stream speechpb.Speech_StreamingRecognizeClient, receiveChan chan bool) {
	// tidy up when function returns
	defer close(receiveChan)
	// don't really want to flush on return here, as it doesn't make sense in recovery
	// scenario. Only want to flush at timer expiry.
	// defer flush()

	// if no results received from Speech for some period, emit any pending transcriptions
	timeout := *flushTimeout * time.Millisecond
	timer := time.NewTimer(timeout)
	go flushOnExpiry(timer)
	// go flushCloseOnExpiry(timer, stream)
	// go closeOnExpiry(timer, stream)
	defer timer.Stop()

	// consume streaming responses from Speech API
	for {
		resp, err := stream.Recv()
		if err != nil {
			// Context cancelled - expected, e.g. stopped being leader
			if status.Code(err) == codes.Canceled {
				return
			}
			klog.Errorf("Cannot stream results: %v", err)
			return
		}
		if err := resp.Error; err != nil {
			// timeout - expected, when no audio sent for a time
			if status.FromProto(err).Code() == codes.OutOfRange {
				klog.Info("Timeout from API; closing connection")
				return
			}
			klog.Errorf("Could not recognize: %v", err)
			return
		}
		if !timer.Stop() {
			klog.Info("Timer expired, returning from receive")
			return
		}

		// ok, we have a valid response from Speech.
		timer.Reset(timeout)
		// if !timer.Reset(timeout) {
		// 	// go flushOnExpiry(timer)
		// 	// go flushCloseOnExpiry(timer, stream)
		// 	go closeOnExpiry(timer, stream)
		// }
		processResponses(*resp)
	}
}

// Handles transcription results.
// Broadly speaking, the goal is to emit 1-3 trancribed words at a time.
// An index is maintained to determine last emitted words.
// Emits a delimited string with 3 parts:
// 'steady' - these words are assumed final (won't change)
// 'pending' - interim transcriptions often evolve as the API gets more data,
// 		so the most recently transcribed words should be treated with caution
// 'unstable' - low-stability transcritpion alternatives
func processResponses(resp speechpb.StreamingRecognizeResponse) {
	if len(resp.Results) == 0 {
		return
	}
	// logResponses(resp)
	result := resp.Results[0]
	alternative := result.Alternatives[0]
	latestTranscript = alternative.Transcript
	elements := strings.Split(alternative.Transcript, " ")
	length := len(elements)

	// Speech will not further update this transcription; output it
	if result.GetIsFinal() || alternative.GetConfidence() > 0 {
		klog.Info("Final result! Resetting")
		final := elements[lastIndex:]
		emit(strings.Join(final, " "))
		reset()
		return
	}
	if result.Stability < 0.75 {
		klog.Infof("Ignoring low stability result (%v): %s", resp.Results[0].Stability,
			resp.Results[0].Alternatives[0].Transcript)
		return
	}

	// lower confidence results
	if len(resp.Results) > 1 {
		unstable = resp.Results[1].Alternatives[0].Transcript
	}
	// Treat last N words as pending.
	if length < *pendingWordCount {
		lastIndex = 0
		pending = elements
		emitStages([]string{}, pending, unstable)
	} else if lastIndex < length-*pendingWordCount {
		steady := elements[lastIndex:(length - *pendingWordCount)]
		lastIndex += len(steady)
		pending = elements[lastIndex:]
		emitStages(steady, pending, unstable)
	}
}

func initStreamingRequest(ctx context.Context, client *speech.Client, config speechpb.StreamingRecognitionConfig) speechpb.Speech_StreamingRecognizeClient {
	stream, err := client.StreamingRecognize(ctx)
	if err != nil {
		klog.Fatal(err)
	}
	// Send the initial configuration message.
	if err := stream.Send(&speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &config,
		},
	}); err != nil {
		klog.Errorf("Error sending initial config message: %v", err)
		return nil
	}
	klog.Info("Initialised new connection to Speech API")
	return stream
}

func emitStages(steady []string, pending []string, unstable string) {
	// concatenate the different segments, let client decide how to display
	msg := fmt.Sprintf("%s|%s|%s", strings.Join(steady, " "),
		strings.Join(pending, " "), unstable)
	emit(msg)
}

func emit(msg string) {
	klog.Info(msg)
	redisClient.LPush(*transcriptionQueue, msg)
}

func flushOnExpiry(timer *time.Timer) {
	<-timer.C
	flush()
}

func flushCloseOnExpiry(timer *time.Timer, stream speechpb.Speech_StreamingRecognizeClient) {
	<-timer.C
	flush()
	stream.CloseSend()
}

func closeOnExpiry(timer *time.Timer, stream speechpb.Speech_StreamingRecognizeClient) {
	<-timer.C
	stream.CloseSend()
}

func flush() {
	msg := ""
	if pending != nil {
		msg += strings.Join(pending, " ")
	}
	if unstable != "" && !strings.HasSuffix(msg, unstable) {
		msg += unstable
	}
	if msg != "" {
		klog.Info("Flushing...")
		emit("[FLUSH] " + msg)
	}
	reset()
}

func reset() {
	lastIndex = 0
	pending = nil
	unstable = ""
}

// debug
func logResponses(resp speechpb.StreamingRecognizeResponse) {
	for _, result := range resp.Results {
		klog.Infof("Result: %+v\n", result)
	}
}
