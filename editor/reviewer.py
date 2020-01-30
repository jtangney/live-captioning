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
parser.add_argument("--id", default="Reviewer")
args = parser.parse_args()

app = Flask(__name__)
app.config['SECRET_KEY'] = 'secret!'
socketio = SocketIO()
# rdb = redis.Redis(host=args.redisHost, port=6379, db=0, health_check_interval=3)
rdb = redis.Redis(host=args.redisHost, port=6379, db=0)

connected = False
task_running = False


@app.route('/')
def index():
  return render_template('index.html', async_mode=socketio.async_mode)


@socketio.on('connect')
def connect():
  print('%s socket connected!' % args.id)
  global connected, task_running
  # if not connected:
  connected = True
  if not task_running:
    print('Starting background task to read from Redis')
    socketio.start_background_task(target=_read)
    task_running = True


@socketio.on('disconnect')
def disconnect():
  print('%s socket disconnected!' % args.id)
  global connected
  connected = False


def _read():
  while connected:
    try:
      fragment = rdb.brpop(args.redisChannel, timeout=2)
      if fragment is not None:
        socketio.emit('transcript', fragment[1].decode('utf-8'))
    except redis.exceptions.RedisError as err:
      print('Exception reading from Redis: %s' % err)
      socketio.sleep(1)
  print('Exiting background task, no longer reading from Redis')
  global task_running
  task_running = False


@socketio.on('message')
def handle_message(message):
  print('Ignoring received string message: ' + message)


if __name__ == '__main__':
  print('Starting %s...' % args.id)
  socketio.init_app(app)
  socketio.run(app, host=args.host, port=args.port)
