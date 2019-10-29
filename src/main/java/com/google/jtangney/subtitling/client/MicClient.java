package com.google.jtangney.subtitling.client;

import javax.sound.sampled.AudioFormat;
import javax.sound.sampled.AudioSystem;
import javax.sound.sampled.DataLine;
import javax.sound.sampled.TargetDataLine;
import java.util.concurrent.BlockingQueue;

public class MicClient implements AudioClient {

  private static int BYTES_PER_BUFFER = 6400; // buffer size in bytes
  private BlockingQueue<byte[]> sharedQueue;
  private TargetDataLine targetDataLine;

  public MicClient(BlockingQueue<byte[]> sharedQueue) {
    this.sharedQueue = sharedQueue;
    // SampleRate:16000Hz, SampleSizeInBits: 16, Number of channels: 1, Signed: true,
    // bigEndian: false

    AudioFormat audioFormat = new AudioFormat(16000, 16, 1, true, false);
    // Set the system information to read from the microphone audio stream
    DataLine.Info targetInfo = new DataLine.Info(TargetDataLine.class, audioFormat);
    if (!AudioSystem.isLineSupported(targetInfo)) {
      System.out.println("Microphone not supported");
      System.exit(0);
    }
    try {
      // Target data line captures the audio stream the microphone produces.
      targetDataLine = (TargetDataLine) AudioSystem.getLine(targetInfo);
      targetDataLine.open(audioFormat);
    }
    catch (Exception e) {
      throw new RuntimeException(e);
    }

  }

  @Override
  public void run() {
    System.out.printf("Starting to collect audio from microphone...");
    targetDataLine.start();
    System.out.println("Start speaking...Press Ctrl-C to stop");
    byte[] data = new byte[BYTES_PER_BUFFER];
    while (targetDataLine.isOpen()) {
      try {
        int numBytesRead = targetDataLine.read(data, 0, data.length);
        if ((numBytesRead <= 0) && (targetDataLine.isOpen())) {
          continue;
        }
        sharedQueue.put(data.clone());
      } catch (InterruptedException e) {
        System.out.println("Microphone input buffering interrupted : " + e.getMessage());
      }
    }
  }

  @Override
  public boolean isOpen() {
    return targetDataLine.isOpen();
  }
}
