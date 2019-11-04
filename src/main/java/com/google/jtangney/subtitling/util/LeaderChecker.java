package com.google.jtangney.subtitling.util;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.Map;

public class LeaderChecker {

  private static final HttpClient httpClient = HttpClient.newBuilder()
      .version(HttpClient.Version.HTTP_2).build();

  private static String podname;
  private static String port;

  private LeaderChecker(){}

  public static String getLeader() {
    try {
      HttpRequest request = HttpRequest.newBuilder().GET().uri(
          URI.create(String.format("http://localhost:%s/", getPort()))).build();
      HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
      return response.body();
    }
    catch (Exception e) {
      System.out.println(e);
      throw new RuntimeException(e);
    }
  }

  public static boolean isLeader() {
    if (getPort() == null) {
      return true;
    }
    return getLeader().contains(getPodName());
  }

  private static String getPodName() {
    if (podname == null) {
      Map<String, String> env = System.getenv();
      if (env.containsKey("PODNAME")) {
        podname = env.get("PODNAME");
      }
    }
    return podname;
  }

  private static String getPort() {
    if (port == null) {
      Map<String, String> env = System.getenv();
      if (env.containsKey("LEADERPORT")) {
        port = env.get("LEADERPORT");
      }
    }
    return port;
  }
}
