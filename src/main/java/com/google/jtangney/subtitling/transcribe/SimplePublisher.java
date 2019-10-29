package com.google.jtangney.subtitling.transcribe;

/**
 * Publisher impl that simply writes to std out
 */
public class SimplePublisher implements Publisher {

  @Override
  public void publish(String msg) {
    System.out.printf("%s%n", msg);
  }
}
