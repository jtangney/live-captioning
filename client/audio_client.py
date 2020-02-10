import argparse
import asyncio
import time
import wave
from datetime import datetime

import pyaudio
import socketio

parser = argparse.ArgumentParser()
parser.add_argument("--targetip", default="localhost:8080")
parser.add_argument("--file", default="pager-article-snippet.wav")
args = parser.parse_args()

sio = socketio.Client(reconnection_delay=1, reconnection_delay_max=1,
                      randomization_factor=0, logger=False)


@sio.event
def connect():
  print("Socket connected at %s" % datetime.utcnow())


@sio.event
def disconnect():
  print("Socket disconnected at %s" % datetime.utcnow())


@sio.on('pod_id')
def pod_id(msg):
  print('Connected to pod: %s' % msg)


async def stream_mic_audio(target_ip):
  """Streams audio from the local microphone via socketio"""
  pa = pyaudio.PyAudio()
  try:
    pa.get_default_input_device_info()
  except OSError:
    print('No audio input devices found! Aborting!')
    return

  rate = 16000
  chunk = int(rate / 10)
  stream = pa.open(format=pyaudio.paInt16,
                   channels=1,
                   rate=rate,
                   input=True,
                   frames_per_buffer=chunk)
  url = 'http://' + target_ip
  sio.connect(url)
  while sio.connected:
    data = stream.read(chunk)
    sio.emit('data', data)


async def stream_file(target_ip, filename):
  """Streams the supplied WAV file via socketio, continuously replaying"""
  wf = wave.open(filename, 'rb')
  pa = pyaudio.PyAudio()
  try:
    outstream = pa.open(format=pa.get_format_from_width(wf.getsampwidth()),
                        channels=wf.getnchannels(),
                        rate=wf.getframerate(),
                        output=True)
  except OSError:
    print('No output audio devices found! Running in Cloud Shell?')
    outstream = None

  # read in ~100ms chunks
  chunk = int(wf.getframerate() / 10)
  data = wf.readframes(chunk)
  url = 'http://' + target_ip
  sio.connect(url)
  while True:
    try:
      # sio.connect(url)
      while sio.connected:
        if data != '' and len(data) != 0:
          sio.emit('data', data)
          if outstream:
            outstream.write(data)
          else:
            # no output audio devices (cloud shell) - sleep for
            # the duration of the audio chunk (workaround)
            time.sleep(0.1)
          data = wf.readframes(chunk)
        else:
          print('EOF, pausing')
          time.sleep(0.5)
          wf = wave.open(filename, 'rb')
          data = wf.readframes(chunk)
          print('restarting playback')
      # print('socketio not connected!')
    except socketio.exceptions.ConnectionError as err:
      print('Connection error: %s! Retrying at %s' % (err, datetime.utcnow()))


asyncio.get_event_loop().run_until_complete(stream_file(
  args.targetip, args.file))
