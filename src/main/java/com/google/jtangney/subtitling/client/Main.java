package com.google.jtangney.subtitling.client;

import com.google.protobuf.ByteString;
import org.eclipse.jetty.websocket.api.Session;
import org.eclipse.jetty.websocket.client.ClientUpgradeRequest;
import org.eclipse.jetty.websocket.client.WebSocketClient;

import java.net.URI;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.Future;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;

public class Main {

  // Creating shared object
  private static volatile BlockingQueue<byte[]> sharedQueue = new LinkedBlockingQueue();

  public static void main(String[] args) {
    WebSocketHandler socket = new WebSocketHandler();
    try {
      WebSocketClient client = new WebSocketClient();
      client.start();
      // URI uri = new URI("ws://35.244.194.43:80/transcribe");
      URI uri = new URI("ws://localhost:8080/transcribe");
      ClientUpgradeRequest request = new ClientUpgradeRequest();
      System.out.printf("Connecting to : %s%n", uri);
      Future<Session> session = client.connect(socket, uri, request);
      // block until we're connected
      session.get(5, TimeUnit.SECONDS);
    }
    catch (Exception e) {
      throw new RuntimeException("Problem setting up websocket", e);
    }

    MicrophoneClient mic = new MicrophoneClient(sharedQueue);
    System.out.printf("Starting to collect audio from microphone...");
    mic.start();

    long startTime = System.currentTimeMillis();
    ByteString tempByteString;
    try {
      while (true) {
        long estimatedTime = System.currentTimeMillis() - startTime;
        // read some audio bytes from the queue
        tempByteString = ByteString.copyFrom(sharedQueue.take());
        socket.send(tempByteString.toByteArray());
      }
    }
    catch (Exception e) {
      System.exit(1);
    }

  }
}
