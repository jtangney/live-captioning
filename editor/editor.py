import argparse

import eventlet

eventlet.monkey_patch()
from flask import Flask, render_template
from flask_socketio import SocketIO
import redis

parser = argparse.ArgumentParser()
parser.add_argument("--host", default="localhost")
parser.add_argument("--port", default=8080)
parser.add_argument("--redisHost", required=True)
parser.add_argument("--redisChannel", default="transcriptions")
parser.add_argument("--id", default="Editor")
args = parser.parse_args()

app = Flask(__name__)
app.config['SECRET_KEY'] = 'secret!'
socketio = SocketIO()
# rdb = redis.Redis(host=args.redisHost, port=6379, db=0, health_check_interval=3)
rdb = redis.Redis(host=args.redisHost, port=6379, db=0, socket_timeout=4)
p = rdb.pubsub()
subscribed = False
thread = None


@app.route('/')
def index():
  return render_template('index.html', async_mode=socketio.async_mode)


@socketio.on('connect')
def connect():
  print('%s socket connected!' % args.id)
  _init()


@socketio.on('disconnect')
def disconnect():
  print('%s socket disconnected!' % args.id)


def _init():
  global subscribed
  if not subscribed:
    try:
      p.subscribe(args.redisChannel)
      subscribed = True
      global thread
      thread = socketio.start_background_task(target=_pubsub_listen)
      print('started new background task')
    except Exception as err:
      print('Exception subscribing to Redis channel: %s' % err)
      _reconnect()


def _pubsub_listen(sleep=0.1):
  try:
    for msg in p.listen():
      # print('Read pubsub msg:' + str(msg))
      if 'subscribe' not in msg['type']:
        payload = msg['data'].decode('utf-8')
        socketio.emit('pubsubmsg', payload)
      socketio.sleep(sleep)
  except redis.exceptions.RedisError as err:
    print('Exception in pubsub listen thread: %s' % err)
    _reconnect()


def _reconnect(sleep=0.5):
  global subscribed
  subscribed = False
  if sleep > 0:
    socketio.sleep(sleep)
  print('Restarting pubsub listen thread...')
  _init()


@socketio.on('message')
def handle_message(message):
  print('Ignoring received string message: ' + message)


if __name__ == '__main__':
  socketio.init_app(app)
  socketio.run(app, host=args.host, port=args.port)
  _init()
