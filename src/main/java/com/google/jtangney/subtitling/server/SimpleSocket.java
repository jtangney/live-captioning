package com.google.jtangney.subtitling.server;

import com.google.api.gax.rpc.BidiStreamingCallable;
import com.google.cloud.speech.v1.*;
import com.google.protobuf.ByteString;
import org.eclipse.jetty.websocket.api.Session;
import org.eclipse.jetty.websocket.api.WebSocketAdapter;

import java.io.IOException;
import java.util.Arrays;
import java.util.List;
import java.util.logging.Level;
import java.util.logging.Logger;

public class SimpleSocket extends WebSocketAdapter {

  private static final Logger logger = Logger.getLogger(SimpleSocket.class.getName());

  /**
   * Called when the client sends this server some raw bytes (ie audio data).
   */
  @Override
  public void onWebSocketBinary(byte[] payload, int offset, int len) {
    if (isConnected()) {
      logger.info(String.format("Got binary"));
    }
  }

  /**
   * Called when the client sends this server some text.
   */
  @Override
  public void onWebSocketText(String message) {
    if (isConnected()) {
      logger.info(String.format("Got text: %s", message));
    }
  }

  public void onWebSocketConnect(Session sess) {
    logger.info("Websocket connect!!");
    super.onWebSocketConnect(sess);
  }

  /**
   * Called when the connection to the client is closed.
   */
  @Override
  public void onWebSocketClose(int statusCode, String reason) {
    logger.info("Websocket close.");
    super.onWebSocketClose(statusCode, reason);
  }

  /**
   * Called if there's an error connecting with the client.
   */
  @Override
  public void onWebSocketError(Throwable cause) {
    logger.log(Level.WARNING, "Websocket error", cause);
    super.onWebSocketError(cause);
  }

}
