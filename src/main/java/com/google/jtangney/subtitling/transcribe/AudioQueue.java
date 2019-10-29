package com.google.jtangney.subtitling.transcribe;

public interface AudioQueue {

  byte[] take();

  boolean isEmpty();

  int size();
}
