/**
 * Copyright 2017 Google Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package com.google.jtangney.subtitling.ingest;

import org.eclipse.jetty.websocket.api.Session;
import org.eclipse.jetty.websocket.api.WebSocketAdapter;

import java.util.Arrays;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.logging.Level;
import java.util.logging.Logger;

@Deprecated
public class TranscribeSocket extends WebSocketAdapter {

  private static final Logger logger = Logger.getLogger(TranscribeSocket.class.getName());

  private static volatile BlockingQueue<byte[]> sharedQueue = new LinkedBlockingQueue();
//  private SpeechHandler speechHandler;


  public TranscribeSocket() {
  }

  /**
   * Called when the client initiates WebSocket connection.
   * In response, initialise the Speech API
   */
  public void onWebSocketConnect(Session sess) {
    logger.info("Websocket connect!!");
    super.onWebSocketConnect(sess);
//    speechHandler = new SpeechHandler(sharedQueue, sess);
//    new Thread(speechHandler).start();
  }

  /**
   * Called when the client sends this transcribe some raw bytes (ie audio data).
   */
  @Override
  public void onWebSocketBinary(byte[] payload, int offset, int len) {
    if (isConnected()) {
      sharedQueue.add(Arrays.copyOfRange(payload, offset, len));
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
//    speechHandler.onCompleted();
  }

  /**
   * Called if there's an error connecting with the client.
   */
  @Override
  public void onWebSocketError(Throwable cause) {
    logger.log(Level.WARNING, "Websocket error", cause);
//    speechHandler.onError(cause);
  }

}
