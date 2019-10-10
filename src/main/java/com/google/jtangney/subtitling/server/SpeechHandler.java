package com.google.jtangney.subtitling.server;

import com.google.api.gax.rpc.ApiStreamObserver;
import com.google.api.gax.rpc.BidiStreamingCallable;
import com.google.cloud.speech.v1.*;
import com.google.protobuf.ByteString;
import com.google.protobuf.Duration;
import org.eclipse.jetty.websocket.api.Session;

import java.io.IOException;
import java.text.DecimalFormat;
import java.util.Arrays;
import java.util.List;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.logging.Level;
import java.util.logging.Logger;

public class SpeechHandler implements Runnable, ApiStreamObserver<StreamingRecognizeResponse> {

  private static final Logger logger = Logger.getLogger(SpeechHandler.class.getName());

  private ApiStreamObserver<StreamingRecognizeRequest> requestObserver;
  private SpeechClient speech;
  private BlockingQueue<byte[]> sharedQueue;
  private Session wsSession;
  private final AtomicBoolean running = new AtomicBoolean(false);

  public static final String RED = "\033[0;31m";
  public static final String GREEN = "\033[0;32m";
  public static final String YELLOW = "\033[0;33m";

  private static final int STREAMING_LIMIT = 290000; // ~5 minutes

  private static int restartCounter = 0;
  //  private static ArrayList<ByteString> audioInput = new ArrayList<ByteString>();
  //  private static ArrayList<ByteString> lastAudioInput = new ArrayList<ByteString>();
  private static int resultEndTimeInMS = 0;
  private static int isFinalEndTime = 0;
  private static int finalRequestEndTime = 0;
  private static boolean newStream = true;
  private static double bridgingOffset = 0;
  private static boolean lastTranscriptWasFinal = false;


  SpeechHandler(BlockingQueue<byte[]> sharedQueue, Session sess) {
    this.sharedQueue = sharedQueue;
    this.wsSession = sess;
    this.initSpeech();
  }

  private void initSpeech() {
    try {
      speech = SpeechClient.create();
      BidiStreamingCallable<StreamingRecognizeRequest, StreamingRecognizeResponse> callable =
          speech.streamingRecognizeCallable();

      requestObserver = callable.bidiStreamingCall(this);
      // Build and send a StreamingRecognizeRequest containing the parameters for
      // processing the audio.
      RecognitionConfig config =
          RecognitionConfig.newBuilder()
              .setEncoding(RecognitionConfig.AudioEncoding.LINEAR16)
              .setSampleRateHertz(16000)
              .setLanguageCode("en-US")
              .build();
      StreamingRecognitionConfig streamingConfig =
          StreamingRecognitionConfig.newBuilder()
              .setConfig(config)
              .setInterimResults(true)
              .setSingleUtterance(false)
              .build();

      StreamingRecognizeRequest initial =
          StreamingRecognizeRequest.newBuilder().setStreamingConfig(streamingConfig).build();
      requestObserver.onNext(initial);
      logger.info("Initialised streaming Speech API");
    } catch (IOException e) {
      logger.log(Level.WARNING, "Error onWebSocketText", e);
    }
  }

  @Override
  public void run() {
    running.set(true);
    try {
      while (running.get()) {
        // read some audio bytes from the queue
        ByteString tempByteString = ByteString.copyFrom(sharedQueue.take());
        StreamingRecognizeRequest request =
            StreamingRecognizeRequest.newBuilder()
                .setAudioContent(tempByteString)
                .build();
        requestObserver.onNext(request);
      }
    }
    catch (Exception e) {
      throw new RuntimeException(e);
    }
  }

  public void stop() {
    this.running.set(false);
  }

  /**
   * Called when the Speech API has a transcription result for us.
   */
  @Override
  public void onNext(StreamingRecognizeResponse response) {
    List<StreamingRecognitionResult> results = response.getResultsList();
    if (results.size() < 1) {
      return;
    }

    StreamingRecognitionResult result = response.getResultsList().get(0);
    Duration resultEndTime = result.getResultEndTime();
    resultEndTimeInMS = (int) ((resultEndTime.getSeconds() * 1000)
        + (resultEndTime.getNanos() / 1000000));
    double correctedTime = resultEndTimeInMS - bridgingOffset
        + (STREAMING_LIMIT * restartCounter);

    SpeechRecognitionAlternative alternative = result.getAlternativesList().get(0);
    if (result.getIsFinal()) {
      System.out.print(GREEN);
      System.out.print("\033[2K\r");
      System.out.printf("%s: %s [confidence: %.2f]\n",
          convertMillisToDate(correctedTime),
          alternative.getTranscript(),
          alternative.getConfidence()
      );
      isFinalEndTime = resultEndTimeInMS;
      lastTranscriptWasFinal = true;
    }
    else {
      System.out.print(RED);
      System.out.print("\033[2K\r");
      System.out.printf("%s: %s", convertMillisToDate(correctedTime),
          alternative.getTranscript()
      );
      lastTranscriptWasFinal = false;
    }
  }

//  @Deprecated
//  void origHandleResponse(StreamingRecognizeResponse response) {
//    List<StreamingRecognitionResult> results = response.getResultsList();
//    if (results.size() < 1) {
//      return;
//    }
//
//    try {
//      StreamingRecognitionResult result = results.get(0);
//      logger.info("Got result " + result);
//      //String transcript = result.getAlternatives(0).getTranscript();
//      getRemote().sendString(gson.toJson(result));
//    } catch (IOException e) {
//      logger.log(Level.WARNING, "Error sending to websocket", e);
//    }
//  }

  /**
   * Called if the API call throws an error.
   */
  @Override
  public void onError(Throwable error) {
    logger.log(Level.WARNING, "recognize failed", error);
    // Close the websocket
    wsSession.close(500, error.toString());
    stop();
    speech.close();
  }

  /**
   * Called when the API call is complete.
   */
  @Override
  public void onCompleted() {
    logger.info("recognize completed.");
    // Close the websocket
    wsSession.close();
    stop();
    speech.close();
  }


  // Taken wholesale from StreamingRecognizeClient.java
  private static final List<String> OAUTH2_SCOPES =
      Arrays.asList("https://www.googleapis.com/auth/cloud-platform");

  private String convertMillisToDate(double milliSeconds) {
    long millis = (long) milliSeconds;
    DecimalFormat format = new DecimalFormat();
    format.setMinimumIntegerDigits(2);
    return String.format("%s:%s /",
        format.format(TimeUnit.MILLISECONDS.toMinutes(millis)),
        format.format(TimeUnit.MILLISECONDS.toSeconds(millis)
            - TimeUnit.MINUTES.toSeconds(TimeUnit.MILLISECONDS.toMinutes(millis)))
    );
  }

}
