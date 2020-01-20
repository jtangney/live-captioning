import argparse
import asyncio
import time
import wave

import pyaudio
import socketio
import sounddevice

parser = argparse.ArgumentParser()
parser.add_argument("--targetip", default="localhost:8080")
parser.add_argument("--file", default="pager-article-snippet.wav")
args = parser.parse_args()

sio = socketio.Client()


@sio.event
def connect():
  print("Established socket connection!")


@sio.event
def disconnect():
  print("Socket disconnected!")


async def stream_mic_audio(target_ip):
  """Streams audio from the local microphone via socketio"""
  input_devices = sounddevice.query_devices(kind='input')
  if not input_devices:
    print('No audio input devices found! Aborting!')
    return

  rate = 16000
  chunk = int(rate / 10)
  stream = pyaudio.PyAudio().open(format=pyaudio.paInt16,
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
  output_devices = sounddevice.query_devices(kind='output')
  if not output_devices:
    print('No output audio devices found! Perhaps running in Cloud Shell? Working around')
    outstream = None
  else:
    pa = pyaudio.PyAudio()
    outstream = pa.open(format=pa.get_format_from_width(wf.getsampwidth()),
                        channels=wf.getnchannels(),
                        rate=wf.getframerate(),
                        output=True)
  # read in ~100ms chunks
  chunk = int(wf.getframerate() / 10)
  data = wf.readframes(chunk)
  url = 'http://' + target_ip
  while True:
    try:
      sio.connect(url)
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
          time.sleep(1.0)
          wf = wave.open(filename, 'rb')
          data = wf.readframes(chunk)
          print('restarting playback')
    except socketio.exceptions.ConnectionError as err:
      print('Connection error! Retrying...: %s' % err)
      time.sleep(0.5)


asyncio.get_event_loop().run_until_complete(stream_file(
  args.targetip, args.file))
