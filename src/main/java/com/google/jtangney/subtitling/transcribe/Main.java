package com.google.jtangney.subtitling.transcribe;

import com.google.jtangney.subtitling.ingest.IngestSocket;

/**
 * Entry point for the Transcriber service.
 * Initialises audio consumer.
 */
public class Main {

  public static void main(String[] args) {
    AudioQueue queue = new JedisAudioQueue(IngestSocket.LIVE_QUEUE, null);
    SpeechHandler handler = new SpeechHandler(queue);
    new Thread(handler).start();
  }
}
