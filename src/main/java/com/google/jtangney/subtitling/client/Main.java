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
    String host = System.getProperty("socketServer");
    System.out.println(host);
    WebSocketHandler socket = null;
    try {
      socket = websocketConnect(host);
    }
    catch (Exception e) {
      e.printStackTrace();
      System.exit(1);
    }

    MicrophoneClient mic = new MicrophoneClient(sharedQueue);
    System.out.printf("Starting to collect audio from microphone...");
    mic.start();

    long startTime = System.currentTimeMillis();
    ByteString tempByteString;
    try {
      while (true) {
        if (socket.isConnected()) {
          long estimatedTime = System.currentTimeMillis() - startTime;
          // read some audio bytes from the queue
          tempByteString = ByteString.copyFrom(sharedQueue.take());
          socket.send(tempByteString.toByteArray());
        }
        else {
          System.out.println("WebSocket not connected! Retrying connection...");
          try {
            socket = websocketConnect(host);
          }
          catch (Exception e) {
            System.out.println("Failed to connect: "+ e.getMessage());
          }
        }
      }
    }
    catch (Exception e) {
      System.exit(1);
    }

  }

  static WebSocketHandler websocketConnect(String host) throws Exception {
    WebSocketHandler socket = new WebSocketHandler();
    WebSocketClient client = new WebSocketClient();
    client.start();
    URI uri = new URI(String.format("ws://%s/transcribe", host));
    ClientUpgradeRequest request = new ClientUpgradeRequest();
    System.out.printf("Connecting to : %s%n", uri);
    Future<Session> session = client.connect(socket, uri, request);
    // block until we're connected
    session.get(5, TimeUnit.SECONDS);
    return socket;
  }
}
