import asyncio
import time
import wave

import pyaudio
import socketio

RATE = 16000
CHUNK = int(RATE / 10)
SLEEP = 0.5

audio = pyaudio.PyAudio()
sio = socketio.Client()


@sio.event
def disconnect():
  print("Disconnected!")


@sio.event
def connect():
  print("Established connection!")


async def stream_mic_audio(url):
  """Streams audio from the local microphone via socketio"""
  stream = audio.open(format=pyaudio.paInt16,
                      channels=1,
                      rate=RATE,
                      input=True,
                      frames_per_buffer=CHUNK)
  sio.connect(url)
  while sio.connected:
    data = stream.read(CHUNK)
    sio.emit('data', data)


async def stream_file(url, filename='pager-article-snippet.wav'):
  """Streams the supplied file via socketio, continuously replaying"""
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
      print('Connection error! Retrying...')
      time.sleep(SLEEP)


# asyncio.get_event_loop().run_until_complete(stream_file('http://0.0.0.0:8080',
#     filename='/Users/jtangney/Downloads/sky-audio-samples/Weather-mod.wav'))
asyncio.get_event_loop().run_until_complete(stream_file('http://35.241.200.226',
      filename='/Users/jtangney/Downloads/sky-audio-samples/Weather-mod.wav'))
