package com.google.jtangney.subtitling.transcribe;

import com.google.jtangney.subtitling.ingest.IngestSocket;

public class Main {

  public static void main(String[] args) {
    AudioQueue queue = new JedisAudioQueue(IngestSocket.QUEUE_NAME);
    SpeechHandler handler = new SpeechHandler(queue);
    new Thread(handler).start();
  }
}
