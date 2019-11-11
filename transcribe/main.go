package main

// [START speech_transcribe_streaming_mic]
import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	speech "cloud.google.com/go/speech/apiv1p1beta1"
	redis "github.com/go-redis/redis/v7"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1p1beta1"
)

const wordSettleLength int = 4

var stream speechpb.Speech_StreamingRecognizeClient
var lastIndex = 0
var latestTranscript string
var unsent []string
var unstable string

func main() {
	var isNewStream bool
	var redisHost string
	var electionID string
	var electionPort int
	var leaderOnly bool
	flag.StringVar(&redisHost, "redisHost", "localhost", "Redis host IP")
	flag.StringVar(&electionID, "electionID", "", "ID for this candidate in leader election")
	flag.IntVar(&electionPort, "electionPort", 4040, "Local port to check leader ID")
	flag.BoolVar(&leaderOnly, "leaderOnly", true, "Whether to transcribe only if leader")
	flag.Parse()
	if !leaderOnly {
		log.Print("Not doing leader election")
	}
	if leaderOnly && electionID == "" {
		log.Fatalf("Must specify ID for leader election")
	}

	leaderURL := fmt.Sprintf("http://localhost:%d", electionPort)
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisHost + ":6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	ctx := context.Background()
	client, err := speech.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}

	receive := func() {
		duration := 1500 * time.Millisecond
		timer := time.NewTimer(duration)
		go func() {
			<-timer.C
			flush()
		}()
		for {
			resp, err := stream.Recv()
			if err != nil {
				log.Printf("Cannot stream results: %v", err)
				close()
				return
			}
			if err := resp.Error; err != nil {
				log.Printf("Could not recognize: %v", err)
				close()
				return
			}
			timer.Reset(duration)
			if len(resp.Results) > 0 {
				if resp.Results[0].Stability < 0.89 {
					log.Printf("Ignoring low stability result (%v)", resp.Results[0].Stability)
					continue
				}
				// printAllResults(*resp)
				printIncremental(*resp)
			}
		}
	}

	for {
		if leaderOnly && !isLeader(electionID, leaderURL) {
			continue
		}
		result, err := redisClient.BRPop(0, "liveq").Result()
		if err != nil {
			log.Printf("Could not read from liveq: %v", err)
			continue
		}
		isNewStream = maybeInitSteamingRequest(ctx, client)
		if isNewStream {
			go receive()
		}
		decoded, _ := base64.StdEncoding.DecodeString(result[1])
		if err := stream.Send(&speechpb.StreamingRecognizeRequest{
			StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
				AudioContent: decoded,
			},
		}); err != nil {
			log.Printf("Could not send audio: %v", err)
		}
	}
}

// func maybeInitSteamingRequest(stream speechpb.Speech_StreamingRecognizeClient) {
func maybeInitSteamingRequest(ctx context.Context, client *speech.Client) bool {
	if stream != nil {
		return false
	}
	var err error
	stream, err = client.StreamingRecognize(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// Send the initial configuration message.
	if err := stream.Send(&speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					Encoding:                   speechpb.RecognitionConfig_LINEAR16,
					SampleRateHertz:            16000,
					LanguageCode:               "en-US",
					EnableAutomaticPunctuation: true,
				},
				InterimResults: true,
			},
		},
	}); err != nil {
		log.Printf("Error sending initial config message: %v", err)
		return false
	}
	log.Print("Initialised Speech API")
	return true
}

func isLeader(podName string, url string) bool {
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Could not get leader details: %v", err)
		return false
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read leader response: %v", err)
		return false
	}
	return strings.Contains(string(body), podName)
}

func printIncremental(resp speechpb.StreamingRecognizeResponse) {
	result := resp.Results[0]
	alternative := result.Alternatives[0]
	latestTranscript = alternative.Transcript
	elements := strings.Split(alternative.Transcript, " ")
	length := len(elements)

	if result.IsFinal {
		log.Print("Final result! Resetting counts")
		segment := elements[lastIndex:]
		log.Print(strings.Join(segment, " "))
		reset()
		return
	}

	if len(resp.Results) > 1 {
		unstable = resp.Results[1].Alternatives[0].Transcript
	}
	if length < wordSettleLength {
		lastIndex = 0
		unsent = elements
		return
	}
	if lastIndex < length-wordSettleLength {
		segment := elements[lastIndex:(length - wordSettleLength)]
		log.Print(strings.Join(segment, " "))
		lastIndex += len(segment)
		unsent = elements[lastIndex:]
		return
	}
}

func close() {
	flush()
	if err := stream.CloseSend(); err != nil {
		log.Printf("Could not close stream: %v", err)
	}
	stream = nil
}

func flush() {
	if unsent != nil {
		log.Print("Flushing...")
		log.Print(strings.Join(unsent, " "))
	}
	if unstable != "" {
		log.Print(unstable)
	}
	reset()
}

func reset() {
	lastIndex = 0
	unsent = nil
	unstable = ""
}

func printAllResults(resp speechpb.StreamingRecognizeResponse) {
	for _, result := range resp.Results {
		fmt.Printf("Result: %+v\n", result)
	}
}
