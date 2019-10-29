package com.google.jtangney.subtitling.util;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.Map;

public class LeaderChecker {

  public static final String LEADER_URL = "http://localhost:4040";
  private static final HttpClient httpClient = HttpClient.newBuilder()
      .version(HttpClient.Version.HTTP_2).build();

  private static String podname;

  private LeaderChecker(){}

  public static String getLeader() {
    try {
      HttpRequest request = HttpRequest.newBuilder().GET().uri(URI.create(LEADER_URL)).build();
      HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
      return response.body();
    }
    catch (Exception e) {
      return null;
    }
  }

  public static boolean isLeader() {
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
}
