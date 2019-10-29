package com.google.jtangney.subtitling.transcribe;

import com.google.cloud.ByteArray;
import com.google.jtangney.subtitling.util.JedisFactory;
import redis.clients.jedis.Jedis;

import java.util.List;

public class JedisAudioQueue implements AudioQueue {

  private Jedis jedis;
  private String key;

  public JedisAudioQueue(String key) {
    this.jedis = JedisFactory.get();
    this.key = key;
  }

  @Override
  public byte[] take() {
    List<String> value = jedis.blpop(key, "0");
    return ByteArray.copyFrom(value.get(1)).toByteArray();
  }

  @Override
  public boolean isEmpty() {
    return !jedis.exists(key);
  }

  @Override
  public int size() {
    return jedis.llen(key).intValue();
  }
}
