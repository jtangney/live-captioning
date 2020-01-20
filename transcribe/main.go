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
	recoveryQueue      = flag.String("recoveryQueue", "liveq", "Redis key for recent audio data used for recovery")
	flushTimeout       = flag.Duration("flushTimeoutMs", 2000, "Emit any pending transcriptions after this time")
	pendingWordCount   = flag.Int("pendingWordCount", 4, "Treat last N transcribed words as pending")
	encoding           = flag.String("encoding", "LINEAR16", "Audio endcoding for input file")
	sampleRate         = flag.Int("sampleRate", 16000, "Sample rate (Hz)")
	channels           = flag.Int("channels", 1, "Number of audio channels")
	lang               = flag.String("lang", "en-US", "Transcription language code")
	phrases            = flag.String("phrases", "", "Comma-separated list of phrase hints for Speech API. Phrases with spaces should be quoted")

	redisClient      *redis.Client
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

func sendAudio(ctx context.Context, speechClient *speech.Client, config speechpb.StreamingRecognitionConfig) {
	// main loop that consumes the audio data and sends to Speech API
	var stream speechpb.Speech_StreamingRecognizeClient
	done := make(chan bool)
	doRecovery := redisClient.Exists(*recoveryQueue).Val() > 0
	if doRecovery {
		klog.Infof("Data in %s, sending that first", *recoveryQueue)
		emit("[RECOVER]")
	}

	for {
		select {
		case <-ctx.Done():
			klog.Infof("Context cancelled, exiting sender loop")
			return
		case _, ok := <-done:
			if !ok && stream != nil {
				klog.Info("done channel closed, resetting stream")
				stream = nil
				done = make(chan bool)
				continue
			}
		default:
			// process audio
		}

		var result string
		var err error
		if doRecovery {
			result, err = redisClient.RPop(*recoveryQueue).Result()
			if result == "" {
				doRecovery = false
			}
		} else {
			// blocking pop audio data off the queue.
			// Timeout occasionally for hygiene.
			result, err = redisClient.BRPopLPush(*audioQueue, *recoveryQueue, 5*time.Second).Result()
			if err == redis.Nil { // timeout
				continue
			}
			redisClient.Expire(*recoveryQueue, 10*time.Second)
			redisClient.LTrim(*recoveryQueue, 0, 50)
		}
		if err != nil && err != redis.Nil {
			klog.Errorf("Could not read from Redis: %v", err)
			flush()
			continue
		}

		// establish bi-directional connection to Speech API if necessary.
		// If so, start a go routine to listen for responses
		if stream == nil {
			stream = initStreamingRequest(ctx, speechClient, config)
			go receive(stream, done)
			emit("[NEW STREAM]")
		}

		// send data, transcription responses received asynchronously
		// redisClient.Expire("recoveryq", 10 * time.Second)
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

func receive(stream speechpb.Speech_StreamingRecognizeClient, done chan bool) {
	// tidy up when function returns
	defer flush()
	defer close(done)

	// if no results received from Speech for some period, write any pending transcriptions
	timeout := *flushTimeout * time.Millisecond
	timer := time.NewTimer(timeout)
	go func() {
		<-timer.C
		flush()
	}()
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

		// ok, we have a valid response from API.
		timer.Reset(timeout)
		if len(resp.Results) > 0 {
			if resp.Results[0].Stability < 0.75 {
				klog.Infof("Ignoring low stability result (%v): %s", resp.Results[0].Stability,
					resp.Results[0].Alternatives[0].Transcript)
				continue
			}
			handleIncremental(*resp)
		}
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

// Handles transcription results.
// Broadly speaking, the goal is to publish 1-3 trancribed words at a time.
// An index is maintained to determine last published words.
// The transcription result is split into categories:
// 'steady' - these words are assumed final, and can be published
// 'pending' - interim transcriptions can evolve as the API gets more
// 		data, so the last few words should not be published yet
// 'unstable' - low-stability transcritpion alternatives
func handleIncremental(resp speechpb.StreamingRecognizeResponse) {
	result := resp.Results[0]
	alternative := result.Alternatives[0]
	latestTranscript = alternative.Transcript
	elements := strings.Split(alternative.Transcript, " ")
	length := len(elements)

	// API will not further update this transcription; output it
	if result.GetIsFinal() {
		klog.Info("Final result! Resetting")
		final := elements[lastIndex:]
		emit(strings.Join(final, " "))
		reset()
		return
	}

	// new transcription segment. This can happen mid stream
	if length < *pendingWordCount {
		lastIndex = 0
		pending = elements
	} else if lastIndex < length-*pendingWordCount {
		steady := elements[lastIndex:(length - *pendingWordCount)]
		lastIndex += len(steady)
		pending = elements[lastIndex:]
		emitStages(steady, pending, unstable)
	}
	if len(resp.Results) > 1 {
		unstable = resp.Results[1].Alternatives[0].Transcript
	}
}

func emitStages(steady []string, pending []string, unstable string) {
	// concatenate the different stages, let client decide how to display
	msg := fmt.Sprintf("%s|%s|%s", strings.Join(steady, " "),
		strings.Join(pending, " "), unstable)
	emit(msg)
}

func emit(msg string) {
	klog.Info(msg)
	redisClient.LPush(*transcriptionQueue, msg)
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
func printAllResults(resp speechpb.StreamingRecognizeResponse) {
	for _, result := range resp.Results {
		klog.Infof("Result: %+v\n", result)
	}
}
