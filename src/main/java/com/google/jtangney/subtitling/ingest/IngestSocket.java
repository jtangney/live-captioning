package com.google.jtangney.subtitling.ingest;

import com.google.jtangney.subtitling.util.JedisFactory;
import org.eclipse.jetty.websocket.api.Session;
import org.eclipse.jetty.websocket.api.WebSocketAdapter;
import redis.clients.jedis.Jedis;

import java.util.Arrays;
import java.util.Base64;
import java.util.logging.Level;
import java.util.logging.Logger;

public class IngestSocket extends WebSocketAdapter {

  private static Logger logger = Logger.getLogger(IngestSocket.class.getName());
  public static final String QUEUE_NAME = "ingestq";

  private Jedis sharedQueue;

  public IngestSocket() {
    this.sharedQueue = JedisFactory.get();
  }

  /**
   * Called when the client initiates WebSocket connection.
   * In response, initialise the Speech API
   */
  public void onWebSocketConnect(Session sess) {
    logger.info("Websocket connect!!");
    super.onWebSocketConnect(sess);
  }

  /**
   * Called when the client sends this transcribe some raw bytes (ie audio data).
   */
  @Override
  public void onWebSocketBinary(byte[] payload, int offset, int len) {
    if (isConnected()) {
      String enc = Base64.getEncoder().encodeToString(Arrays.copyOfRange(payload, offset, len));
      sharedQueue.rpush(QUEUE_NAME, enc);
//      ByteString byteString = ByteString.copyFrom(Arrays.copyOfRange(payload, offset, len));
//      sharedQueue.rpush(QUEUE_NAME, byteString.toString());
    }
  }

  /**
   * Called when the client sends this transcribe some text.
   */
  @Override
  public void onWebSocketText(String message) {
    if (isConnected()) {
      logger.warning(String.format("Ignoring received text message: %s", message));
    }
  }

  /**
   * Called when the connection to the client is closed.
   */
  @Override
  public void onWebSocketClose(int statusCode, String reason) {
    logger.info("Websocket close.");
  }

  /**
   * Called if there's an error connecting with the client.
   */
  @Override
  public void onWebSocketError(Throwable cause) {
    logger.log(Level.WARNING, "Websocket error", cause);
  }
}
