package com.google.jtangney.subtitling.transcribe;

import redis.clients.jedis.Jedis;

/**
 * Publisher impl that sends to Redis
 */
public class RedisPublisher implements Publisher {

  private static final String CHANNEL_NAME = "transcriptions";

  private Publisher wrapped;
  private Jedis jedis;

  public RedisPublisher() {
    this.jedis = new Jedis("localhost");
    this.wrapped = new SimplePublisher();
  }

  @Override
  public void publish(String msg) {
    jedis.publish(CHANNEL_NAME, msg);
    if (this.wrapped != null) {
      this.wrapped.publish(msg);
    }
  }
}
