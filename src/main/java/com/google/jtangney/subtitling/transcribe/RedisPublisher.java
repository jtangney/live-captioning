package com.google.jtangney.subtitling.transcribe;

import com.google.jtangney.subtitling.util.JedisFactory;
import redis.clients.jedis.Jedis;

/**
 * Publisher impl that sends to Redis
 */
public class RedisPublisher implements Publisher {

  private static final String CHANNEL_NAME = "transcriptions";

  private Publisher wrapped;
  private Jedis jedis;

  public RedisPublisher() {
    this.jedis = JedisFactory.get();
    this.wrapped = new LoggingPublisher();
  }

  @Override
  public void publish(String msg) {
    jedis.publish(CHANNEL_NAME, msg);
    if (this.wrapped != null) {
      this.wrapped.publish(msg);
    }
  }
}
