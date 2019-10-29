package com.google.jtangney.subtitling.transcribe;

import com.google.api.gax.rpc.ApiStreamObserver;
import com.google.api.gax.rpc.BidiStreamingCallable;
import com.google.cloud.speech.v1p1beta1.*;
import com.google.jtangney.subtitling.util.LeaderChecker;
import com.google.protobuf.ByteString;
import com.google.protobuf.Duration;
import org.apache.commons.collections4.map.ListOrderedMap;

import java.io.IOException;
import java.text.DateFormat;
import java.text.DecimalFormat;
import java.text.SimpleDateFormat;
import java.util.Arrays;
import java.util.Base64;
import java.util.Date;
import java.util.List;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.logging.Level;
import java.util.logging.Logger;

public class SpeechHandler implements Runnable, ApiStreamObserver<StreamingRecognizeResponse> {

  private static final Logger logger = Logger.getLogger(SpeechHandler.class.getName());

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


  private ApiStreamObserver<StreamingRecognizeRequest> requestObserver;
  private SpeechClient speech;
  private final AtomicBoolean running = new AtomicBoolean(false);

  private static final int wordSettleLength = 4;
  private int lastIndex = 0;
  private String latestTranscript = null;
  private String[] unsent = null;

  private AudioQueue sharedQueue;
  private Publisher publisher;
  private ListOrderedMap<Long, List<StreamingRecognitionResult>> collatedResults;


  SpeechHandler(AudioQueue sharedQueue) {
    this.sharedQueue = sharedQueue;
//    this.publisher = new RedisPublisher();
    this.publisher = new LoggingPublisher();
  }

  private void initSpeech() {
    try {
      speech = SpeechClient.create();
      lastIndex = 0;
      latestTranscript = null;
      unsent = null;
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
              .setEnableAutomaticPunctuation(true)
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
      logger.log(Level.WARNING, "Error initialising Speech API", e);
    }
  }

  @Override
  public void run() {
    running.set(true);
    try {
      while (running.get()) {
        // abort if we're not the leader (master)
        if (!LeaderChecker.isLeader()) {
          continue;
        }
        // (blocking) read some audio bytes from the queue
        byte[] bytes = Base64.getDecoder().decode(sharedQueue.take());
        // don't init speech until we have some bytes
        if (this.speech == null || speech.isShutdown()) {
          this.initSpeech();
        }
        if (!sharedQueue.isEmpty()) {
          System.out.printf("sharedQueue size = %d%n", sharedQueue.size());
        }

        StreamingRecognizeRequest request =
            StreamingRecognizeRequest.newBuilder()
                .setAudioContent(ByteString.copyFrom(bytes))
                .build();
        requestObserver.onNext(request);
      }
    }
    catch (Exception e) {
      throw new RuntimeException(e);
    }
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
    this.publishIncremental(results);
//    this.publishEachTranscript(results);
//    this.collateResults(results);
//    this.origHandleStreamingResult(results);
  }

  private void publishIncremental(List<StreamingRecognitionResult> results) {
    StreamingRecognitionResult result = results.get(0);
    // ignore if first result is early stage, low confidence
    if (result.getStability() < 0.89) {
      return;
    }

    SpeechRecognitionAlternative alternative = result.getAlternativesList().get(0);
    String transcript = alternative.getTranscript();
    this.latestTranscript = transcript;
    String[] elements = transcript.split(" ");

    if (result.getIsFinal()) {
      System.out.printf("%nFINAL RESULT - resetting counts%n");
      String[] segment = Arrays.copyOfRange(elements, lastIndex, elements.length);
      publisher.publish(String.join(" ", segment));
      this.lastIndex = 0;
      this.unsent = null;
      return;
    }

    if (elements.length <= wordSettleLength) {
      //System.out.printf("Ignoring short result%n");
      flush();
      return;
    }
    if (lastIndex < (elements.length - wordSettleLength)) {
      String[] segment = Arrays.copyOfRange(elements, lastIndex, elements.length - wordSettleLength);
      publisher.publish(String.join(" ", segment));
      unsent = Arrays.copyOfRange(elements, elements.length - wordSettleLength, elements.length);
      lastIndex += segment.length;
    }
  }

  private void publishEachTranscript(List<StreamingRecognitionResult> results) {
    StreamingRecognitionResult result = results.get(0);
    if (result.getStability() < 0.89) {
      return;
    }
    if (result.getIsFinal()) {
      System.out.println("***FINAL***");
    }
    SpeechRecognitionAlternative alternative = result.getAlternativesList().get(0);
    if (!alternative.getTranscript().equalsIgnoreCase(latestTranscript)) {
      latestTranscript = alternative.getTranscript();
      if (results.size() > 1) {
        publisher.publish(String.format("%s [%s]%n", latestTranscript,
            results.get(1).getAlternatives(0).getTranscript()));
      }
      else {
        publisher.publish(String.format("%s%n", latestTranscript));
      }
    }
  }

  private void collateResults(List<StreamingRecognitionResult> results) {
    if (this.collatedResults == null) {
      this.collatedResults = new ListOrderedMap<>();
    }
    this.collatedResults.put(System.currentTimeMillis(), results);
  }

  private void outputCollatedResults() {
    System.out.printf("Received %d result sets%n", collatedResults.size());
    DateFormat df = new SimpleDateFormat("HH:mm:ss.S");
    int i = 0;
    for (Long ts : collatedResults.keySet()) {
      System.out.printf("%n%n%nResult %d, %nreceived at: %s", i++, df.format(new Date(ts)));
      List<StreamingRecognitionResult> current = collatedResults.get(ts);
      System.out.printf("%nStreamingRecognitionResult count = %d", current.size());
      for (StreamingRecognitionResult result : current) {
        System.out.printf("%n===");
        System.out.printf("%nstability = %f", result.getStability());
        System.out.printf("%nAlternatives count = %d", result.getAlternativesList().size());
        for (SpeechRecognitionAlternative alternative : result.getAlternativesList()) {
//          System.out.printf("%nword count = %d", alternative.getWordsCount());
          System.out.printf("%ntranscript = %s", alternative.getTranscript());
        }
      }
    }
  }

  private void origHandleStreamingResult(List<StreamingRecognitionResult> results) {
    StreamingRecognitionResult result = results.get(0);
    Duration resultEndTime = result.getResultEndTime();
    resultEndTimeInMS = (int) ((resultEndTime.getSeconds() * 1000)
        + (resultEndTime.getNanos() / 1000000));
    double correctedTime = resultEndTimeInMS - bridgingOffset
        + (STREAMING_LIMIT * restartCounter);

    SpeechRecognitionAlternative alternative = result.getAlternativesList().get(0);
    if (result.getIsFinal()) {
      logger.info(GREEN);
      logger.info("\033[2K\r");
      logger.info(String.format("%s: %s [confidence: %.2f]\n",
          convertMillisToDate(correctedTime),
          alternative.getTranscript(),
          alternative.getConfidence()
      ));
      isFinalEndTime = resultEndTimeInMS;
      lastTranscriptWasFinal = true;
    }
    else {
      logger.info(RED);
      logger.info("\033[2K\r");
      logger.info(String.format("%s: %s", convertMillisToDate(correctedTime),
          alternative.getTranscript())
      );
      lastTranscriptWasFinal = false;
    }
  }

  /**
   * Called if the API call throws an error.
   */
  @Override
  public void onError(Throwable error) {
    logger.log(Level.WARNING, "recognize failed", error);
    close();
  }

  /**
   * Called when the API call is complete.
   */
  @Override
  public void onCompleted() {
    logger.info("recognize completed.");
    close();
    if (this.collatedResults != null && !this.collatedResults.isEmpty()) {
      outputCollatedResults();
      this.collatedResults.clear();
    }
  }

  private void close() {
    speech.close();
    flush();
    //stop();
  }

  private void flush() {
    if (unsent != null) {
      publisher.publish(String.join(" ", unsent));
      unsent = null;
    }
    lastIndex = 0;
  }

  private void stop() {
    this.running.set(false);
  }

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
