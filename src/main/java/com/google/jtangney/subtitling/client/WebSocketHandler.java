package com.google.jtangney.subtitling.client;

import org.eclipse.jetty.websocket.api.Session;
import org.eclipse.jetty.websocket.api.annotations.*;

import java.io.IOException;
import java.nio.ByteBuffer;

@WebSocket
public class WebSocketHandler {

  private Session session;

  @OnWebSocketConnect
  public void onConnect(Session session) {
    System.out.printf("Got connect: %s%n", session);
    this.session = session;
  }

  @OnWebSocketClose
  public void onClose(int statusCode, String reason) {
    System.out.printf("Connection closed: %d - %s%n", statusCode, reason);
    this.session = null;
  }

  public void close() {
    if (isConnected()) {
      System.out.println("Explicitly closing WebSocket");
      this.session.close();
    }
  }

  @OnWebSocketMessage
  public void onMessage(String msg) {
    System.out.printf("Got msg: %s%n", msg);
  }

  @OnWebSocketError
  public void onError(Throwable cause) {
    System.out.print("WebSocket Error: ");
    cause.printStackTrace(System.out);
  }

  public void send(byte[] chunk) {
    this.send(ByteBuffer.wrap(chunk));
  }

  public void send(ByteBuffer bb) {
    if (!this.isConnected()) {
      System.out.println("Websocket not connected; aborting Send");
      return;
    }
    try {
      this.session.getRemote().sendBytes(bb);
    }
    catch (IOException e) {
      throw new RuntimeException(e);
    }
  }

   boolean isConnected() {
    return this.session != null && this.session.isOpen();
  }
}
