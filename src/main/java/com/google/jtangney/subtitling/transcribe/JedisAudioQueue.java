package com.google.jtangney.subtitling.transcribe;

import com.google.jtangney.subtitling.util.JedisFactory;
import redis.clients.jedis.Jedis;

import java.util.ArrayList;
import java.util.Base64;
import java.util.Collections;
import java.util.List;

public class JedisAudioQueue implements AudioQueue {

  private Jedis jedis;
  private String liveQ;
  private String recentQ;
  private int recentSize = 100;

  public JedisAudioQueue(String liveQ) {
    this(liveQ, null);
  }

  public JedisAudioQueue(String live, String recent) {
    this.jedis = JedisFactory.get();
    this.liveQ = live;
    this.recentQ = recent;
  }

  @Override
  public byte[] take() {
    // blocking
    List<String> value = jedis.brpop(liveQ, "0");
    if (recentQ != null) {
      jedis.lpush(recentQ, value.get(1));
      jedis.ltrim(recentQ, 0, recentSize);
    }
    return Base64.getDecoder().decode(value.get(1));
  }

  @Override
  public List<byte[]> takeRecent() {
    if (recentQ == null) {
      return Collections.EMPTY_LIST;
    }
    List<String> recents = jedis.lrange(recentQ, 0, recentSize);
    if (recents.size() <= 1) {
      return null;
    }
    List<byte[]> list = new ArrayList(recentSize);
    for (int i=1; i<recents.size(); i++) {
      byte[] chunk = Base64.getDecoder().decode(recents.get(i));
      list.add(chunk);
    }
    Collections.reverse(list);
    return list;
  }

  @Override
  public boolean isEmpty() {
    return !jedis.exists(liveQ);
  }

  @Override
  public int size() {
    return jedis.llen(liveQ).intValue();
  }
}
