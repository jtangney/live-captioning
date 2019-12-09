package main

// [START speech_transcribe_streaming_mic]
import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"

	// "log"
	"net/http"
	"strings"
	"time"

	speech "cloud.google.com/go/speech/apiv1p1beta1"
	redis "github.com/go-redis/redis/v7"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1p1beta1"
	"k8s.io/klog"
)

const wordSettleLength int = 4

var stream speechpb.Speech_StreamingRecognizeClient
var redisClient *redis.Client
var lastIndex = 0
var latestTranscript string
var pending []string
var unstable string

func main() {
	klog.InitFlags(nil)

	var isNewStream bool
	var redisHost string
	var electionID string
	var electionPort int
	var leaderOnly bool
	var encoding string
	var sampleRate int
	var channels int
	var lang string
	flag.StringVar(&redisHost, "redisHost", "localhost", "Redis host IP")
	flag.StringVar(&electionID, "electionID", "", "ID for this candidate in leader election")
	flag.IntVar(&electionPort, "electionPort", 4040, "Local port to check leader ID")
	flag.BoolVar(&leaderOnly, "leaderOnly", true, "Whether to transcribe only if leader")
	flag.StringVar(&encoding, "encoding", "LINEAR16", "Audio endcoding for input file")
	flag.IntVar(&sampleRate, "sampleRate", 16000, "Sample rate (Hz)")
	flag.IntVar(&channels, "channels", 1, "Number of audio channels")
	flag.StringVar(&lang, "lang", "en-US", "the transcription language code")
	flag.Parse()
	if !leaderOnly {
		klog.Info("Not doing leader election")
	}
	if leaderOnly && electionID == "" {
		klog.Fatalf("Must specify ID if doing leader election")
	}

	leaderURL := fmt.Sprintf("http://localhost:%d", electionPort)
	redisClient = redis.NewClient(&redis.Options{
		Addr:        redisHost + ":6379",
		Password:    "", // no password set
		DB:          0,  // use default DB
		DialTimeout: 3 * time.Second,
		ReadTimeout: 4 * time.Second,
	})

	ctx := context.Background()
	client, err := speech.NewClient(ctx)
	if err != nil {
		klog.Fatal(err)
	}

	streamingConfig := speechpb.StreamingRecognitionConfig{
		Config: &speechpb.RecognitionConfig{
			Encoding:                   speechpb.RecognitionConfig_LINEAR16,
			SampleRateHertz:            int32(sampleRate),
			AudioChannelCount:          int32(channels),
			LanguageCode:               lang,
			EnableAutomaticPunctuation: true,
		},
		InterimResults: true,
	}

	receive := func() {
		// if we don't receive anything from Speech for some period, write any pending transcriptions
		duration := 2000 * time.Millisecond
		timer := time.NewTimer(duration)
		go func() {
			<-timer.C
			flush()
		}()
		// consume streaming responses from Speech API
		for {
			if stream == nil {
				return
			}
			resp, err := stream.Recv()
			if err != nil {
				klog.Errorf("Cannot stream results: %v", err)
				flushAndClose()
				return
			}
			if err := resp.Error; err != nil {
				// timeout - expected
				if err.Code == 11 {
					klog.Info("Timeout from API; closing")
					close()
					return
				}
				klog.Errorf("Could not recognize: %v", err)
				flushAndClose()
				return
			}
			timer.Reset(duration)
			// output the results
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

	// main loop that consumes the audio data and sends to Speech API
	for {
		// if we're not the leader, abort
		if leaderOnly && !isLeader(electionID, leaderURL) {
			continue
		}
		// blocking pop audio data off the queue
		result, err := redisClient.BRPop(0, "liveq").Result()
		if err != nil {
			klog.Errorf("Could not read from Redis liveq: %v", err)
			flushAndClose()
			continue
		}
		// establish bi-directional connection to Speech API if necessary.
		// If so, start a go routine to listen for responses
		isNewStream = maybeInitSteamingRequest(ctx, client, streamingConfig)
		if isNewStream {
			emit("[NEW STREAM]")
			go receive()
		}
		// send data, transcription responses received asynchronously
		decoded, _ := base64.StdEncoding.DecodeString(result[1])
		if err := stream.Send(&speechpb.StreamingRecognizeRequest{
			StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
				AudioContent: decoded,
			},
		}); err != nil {
			klog.Errorf("Could not send audio: %v", err)
		}
	}
}

// func maybeInitSteamingRequest(stream speechpb.Speech_StreamingRecognizeClient) {
func maybeInitSteamingRequest(ctx context.Context, client *speech.Client, config speechpb.StreamingRecognitionConfig) bool {
	if stream != nil {
		return false
	}
	var err error
	stream, err = client.StreamingRecognize(ctx)
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
		return false
	}
	klog.Info("Initialised new connection to Speech API")
	return true
}

// Returns true if this pod is the Leader.
// Communicates with a leader election sidecar container that exposes the
// current leader ID at the url
func isLeader(podName string, url string) bool {
	resp, err := http.Get(url)
	if err != nil {
		klog.Warningf("Could not get leader details: %v", err)
		return false
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		klog.Warningf("Failed to read leader response: %v", err)
		return false
	}
	return strings.Contains(string(body), podName)
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
	if result.IsFinal {
		klog.Info("Final result! Resetting")
		final := elements[lastIndex:]
		emit(strings.Join(final, " "))
		reset()
		return
	}

	// new transcription segment. This can happen mid stream
	if length < wordSettleLength {
		// flush()
		lastIndex = 0
		pending = elements
	} else if lastIndex < length-wordSettleLength {
		steady := elements[lastIndex:(length - wordSettleLength)]
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
	cmd := redisClient.Publish("transcriptions", msg)
	if cmd.Val() == 0 {
		klog.Warning("No subscibers connected to PubSub channel")
	}
}

func flushAndClose() {
	flush()
	close()
}

func close() {
	if stream != nil {
		if err := stream.CloseSend(); err != nil {
			klog.Warningf("Could not close stream: %v", err)
		}
		stream = nil
	}
	reset()
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

func printAllResults(resp speechpb.StreamingRecognizeResponse) {
	for _, result := range resp.Results {
		fmt.Printf("Result: %+v\n", result)
	}
}
