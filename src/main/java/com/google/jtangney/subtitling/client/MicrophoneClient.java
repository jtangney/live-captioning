package com.google.jtangney.subtitling.client;

import com.google.protobuf.ByteString;

import javax.sound.sampled.AudioFormat;
import javax.sound.sampled.AudioSystem;
import javax.sound.sampled.DataLine;
import javax.sound.sampled.TargetDataLine;
import java.util.concurrent.BlockingQueue;

public class MicrophoneClient {

  private static TargetDataLine targetDataLine;
  private static int BYTES_PER_BUFFER = 6400; // buffer size in bytes
  private static ByteString tempByteString;

  private BlockingQueue<byte[]> sharedQueue;

  public MicrophoneClient(BlockingQueue<byte[]> sharedQueue) {
    this.sharedQueue = sharedQueue;
  }

  void start() {
    // Creating microphone input buffer thread
    MicBuffer micrunnable = new MicBuffer();
    Thread micThread = new Thread(micrunnable);

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
      micThread.start();
    }
    catch (Exception e) {
      throw new RuntimeException(e);
    }
  }

  // Microphone Input buffering
  class MicBuffer implements Runnable {

    public void run() {
      System.out.println("Start speaking...Press Ctrl-C to stop");
      targetDataLine.start();
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
  }

}
