package com.google.jtangney.subtitling.transcribe;

import java.util.Collections;
import java.util.List;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.LinkedBlockingQueue;

public class LocalAudioQueue implements AudioQueue {

  private volatile BlockingQueue<byte[]> queue;

  public LocalAudioQueue() {
    this.queue = new LinkedBlockingQueue();
  }

  @Override
  public byte[] take() {
    try {
      return queue.take();
    }
    catch (InterruptedException e) {
      throw new RuntimeException(e);
    }
  }

  @Override
  public List<byte[]> takeRecent() {
    return Collections.emptyList();
  }

  @Override
  public boolean isEmpty() {
    return queue.isEmpty();
  }

  @Override
  public int size() {
    return queue.size();
  }
}
