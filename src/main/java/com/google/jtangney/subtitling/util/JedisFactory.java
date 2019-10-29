package com.google.jtangney.subtitling.util;

import redis.clients.jedis.Jedis;

public class JedisFactory {

  public static final String PROPERTY_REDIS_HOST = "redis.host";
  private static Jedis instance;

  private JedisFactory() {}

  public static Jedis get() {
    if (instance == null) {
      String sysProp = System.getProperty(PROPERTY_REDIS_HOST);
      String redisHost = "localhost";
      if (sysProp != null) {
        redisHost = sysProp;
      }
      System.out.println("Using redis.host="+redisHost);
      instance = new Jedis(redisHost);
    }
    return instance;
  }
}
