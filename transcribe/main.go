package main

// [START speech_transcribe_streaming_mic]
import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	speech "cloud.google.com/go/speech/apiv1p1beta1"
	redis "github.com/go-redis/redis/v7"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1p1beta1"
)

func main() {
	var redisHost string
	flag.StringVar(&redisHost, "redisHost", "localhost", "Redis host IP")
	flag.Parse()
	log.Print(redisHost)

	ctx := context.Background()

	client, err := speech.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	stream, err := client.StreamingRecognize(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// go func() {
	// 	// Pipe stdin to the API.
	// 	buf := make([]byte, 1024)
	// 	for {
	// 		n, err := os.Stdin.Read(buf)
	// 		if n > 0 {
	// 			maybeInitSteamingRequest(stream)
	// 			if err := stream.Send(&speechpb.StreamingRecognizeRequest{
	// 				StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
	// 					AudioContent: buf[:n],
	// 				},
	// 			}); err != nil {
	// 				log.Printf("Could not send audio: %v", err)
	// 			}
	// 		}
	// 		if err == io.EOF {
	// 			// Nothing else to pipe, close the stream.
	// 			if err := stream.CloseSend(); err != nil {
	// 				log.Fatalf("Could not close stream: %v", err)
	// 			}
	// 			return
	// 		}
	// 		if err != nil {
	// 			log.Printf("Could not read from stdin: %v", err)
	// 			continue
	// 		}
	// 	}
	// }()

	go func() {
		podName := os.Getenv("PODNAME")
		client := redis.NewClient(&redis.Options{
			Addr:     redisHost + ":6379",
			Password: "", // no password set
			DB:       0,  // use default DB
		})
		for {
			if !isLeader(podName, "http://localhost:4040") {
				continue
			}
			result, err := client.BRPop(0, "liveq").Result()
			if err != nil {
				log.Printf("Could not read from liveq: %v", err)
				continue
			}
			maybeInitSteamingRequest(stream)
			decoded, _ := base64.StdEncoding.DecodeString(result[1])
			if err := stream.Send(&speechpb.StreamingRecognizeRequest{
				StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
					AudioContent: decoded,
				},
			}); err != nil {
				log.Printf("Could not send audio: %v", err)
			}
		}
	}()

	for {
		resp, err := stream.Recv()
		if err != nil {
			flush()
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Cannot stream results: %v", err)
		}
		if err := resp.Error; err != nil {
			log.Fatalf("Could not recognize: %v", err)
		}
		if resp.Results[0].Stability < 0.89 {
			log.Print("Ignoring low stability results")
			continue
		}
		// printAllResults(*resp)
		printIncremental(*resp)
	}
}

var initialised bool = false
var wordSettleLength = 4
var lastIndex = 0
var latestTranscript string
var unsent []string
var unstable string

func maybeInitSteamingRequest(stream speechpb.Speech_StreamingRecognizeClient) {
	if !initialised {
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
			log.Fatal(err)
		}
		initialised = true
		log.Print("Initialised Speech API")
	}
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

func flush() {
	log.Print("Flushing...")
	if unsent != nil {
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
