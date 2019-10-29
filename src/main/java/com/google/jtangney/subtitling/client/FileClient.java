package com.google.jtangney.subtitling.client;

import javax.sound.sampled.*;
import java.io.File;
import java.net.URL;
import java.nio.file.Paths;
import java.util.concurrent.BlockingQueue;

public class FileClient implements AudioClient {

  private BlockingQueue<byte[]> sharedQueue;
  private String filepath;
  private AudioInputStream audioInputStream;
  private SourceDataLine line;
  private AudioFormat format;

  public FileClient(BlockingQueue<byte[]> sharedQueue) {
    this(sharedQueue, "/pager-article-snippet.wav");
  }

  public FileClient(BlockingQueue<byte[]> sharedQueue, String inputFilePath) {
    this.sharedQueue = sharedQueue;
    this.filepath = inputFilePath;

    try {
      URL fileurl = this.getClass().getResource(this.filepath);
      File fileIn = Paths.get(fileurl.toURI()).toFile();
      this.audioInputStream = AudioSystem.getAudioInputStream(fileIn);
      AudioFormat formatIn = audioInputStream.getFormat();
      this.format = new AudioFormat(formatIn.getSampleRate(), formatIn.getSampleSizeInBits(),
          formatIn.getChannels(), true, formatIn.isBigEndian());
      System.out.println(format.toString());

      DataLine.Info dinfo = new DataLine.Info(SourceDataLine.class, format);
      this.line = (SourceDataLine) AudioSystem.getLine(dinfo);
      this.line.open(format);
    }
    catch (Exception e) {
      System.out.println("Failed to read input file");
      e.printStackTrace();
      System.exit(1);
    }
  }

  @Override
  public void run() {
    try {
      int bytesPerFrame = format.getFrameSize();
      int numBytes = 1024 * bytesPerFrame;
      numBytes = 16000;
      byte[] data = new byte[numBytes];
      line.start();
      System.out.println("playback started");
      while (line.isOpen()) {
        try {
          int numBytesRead = audioInputStream.read(data, 0, data.length);
          //if ((numBytesRead <= 0) && (line.isOpen())) {
          if (numBytesRead <= 0) {
            System.out.println("no more bytes from input file");
            line.close();
          }
          sharedQueue.put(data.clone());
          Thread.sleep(100);
        }
        catch (InterruptedException e) {
          System.out.println("File input buffering interrupted : " + e.getMessage());
        }
      }
      System.out.println("playback finished");
    }
    catch (Exception e) {
      throw new RuntimeException(e);
    }
  }

  @Override
  public boolean isOpen() {
    return line.isOpen();
  }
}
