import asyncio
import time
import wave

import pyaudio
import websockets

RATE = 16000
CHUNK = int(RATE / 10)
SLEEP = 0.5

audio = pyaudio.PyAudio()


async def stream_mic_audio(url):
  """Streams audio from the local microphone via websockets"""
  stream = audio.open(format=pyaudio.paInt16,
                      channels=1,
                      rate=RATE,
                      input=True,
                      frames_per_buffer=CHUNK)

  async with websockets.connect(url) as websocket:
    while True:
      data = stream.read(CHUNK)
      await websocket.send(data)


async def stream_file(url, filename='pager-article-snippet.wav'):
  """Streams the supplied file via websockets, continuously replaying"""
  wf = wave.open(filename, 'rb')
  outstream = audio.open(format=audio.get_format_from_width(wf.getsampwidth()),
                         channels=wf.getnchannels(),
                         rate=wf.getframerate(),
                         output=True)

  data = wf.readframes(CHUNK)
  while True:
    try:
      async with websockets.connect(url) as websocket:
        while True:
          while data != '' and len(data) != 0:
            outstream.write(data)
            await websocket.send(data)
            data = wf.readframes(CHUNK)
          print('EOF, pausing')
          time.sleep(1.5)
          wf = wave.open(filename, 'rb')
          data = wf.readframes(CHUNK)
          print('restarting playback')
    except websockets.exceptions.ConnectionClosed as err:
      print('Connection closed! Retrying...')
      time.sleep(SLEEP)
    except ConnectionRefusedError as err:
      print('Connection refused! Retrying...')
      time.sleep(SLEEP)


# asyncio.get_event_loop().run_until_complete(
#   stream_mic_audio('ws://35.241.200.226/app/ingest'))

# asyncio.get_event_loop().run_until_complete(
#   stream_file('ws://0.0.0.0:8080/ingest'))
asyncio.get_event_loop().run_until_complete(
  stream_file('ws://35.241.200.226/app/ingest'))
