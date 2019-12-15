import argparse
import asyncio
import time
import wave

import pyaudio
import socketio

parser = argparse.ArgumentParser()
parser.add_argument("--targetip", default="localhost:8080")
parser.add_argument("--file", default="pager-article-snippet.wav")
args = parser.parse_args()

audio = pyaudio.PyAudio()
sio = socketio.Client()

RATE = 16000
CHUNK = int(RATE / 10)
SLEEP = 0.5


@sio.event
def connect():
  print("Established socket connection!")


@sio.event
def disconnect():
  print("Disconnected!")


async def stream_mic_audio(target_ip):
  """Streams audio from the local microphone via socketio"""
  url = 'http://' + target_ip
  stream = audio.open(format=pyaudio.paInt16,
                      channels=1,
                      rate=RATE,
                      input=True,
                      frames_per_buffer=CHUNK)
  sio.connect(url)
  while sio.connected:
    data = stream.read(CHUNK)
    sio.emit('data', data)


async def stream_file(target_ip, filename):
  url = 'http://' + target_ip
  """Streams the supplied WAV file via socketio, continuously replaying"""
  wf = wave.open(filename, 'rb')
  outstream = audio.open(format=audio.get_format_from_width(wf.getsampwidth()),
                         channels=wf.getnchannels(),
                         rate=wf.getframerate(),
                         output=True)
  data = wf.readframes(CHUNK)
  while True:
    try:
      sio.connect(url)
      while sio.connected:
        if data != '' and len(data) != 0:
          sio.emit('data', data)
          outstream.write(data)
          data = wf.readframes(CHUNK)
        else:
          print('EOF, pausing')
          time.sleep(1.0)
          wf = wave.open(filename, 'rb')
          data = wf.readframes(CHUNK)
          print('restarting playback')
    except socketio.exceptions.ConnectionError as err:
      print('Connection error! Retrying...: %s' % err)
      time.sleep(SLEEP)


asyncio.get_event_loop().run_until_complete(stream_file(
  args.targetip, args.file))
