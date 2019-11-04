package com.google.jtangney.subtitling.transcribe;

import java.util.List;

public interface AudioQueue {

  byte[] take();

  List<byte[]> takeRecent();

  boolean isEmpty();

  int size();
}
