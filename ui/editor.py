import eventlet
eventlet.monkey_patch()

import argparse

import redis
from flask import Flask, render_template
from flask_socketio import SocketIO

parser = argparse.ArgumentParser()
parser.add_argument("--host", default="localhost")
parser.add_argument("--port", default=8080)
parser.add_argument("--redisHost", required=True)
parser.add_argument("--redisChannel", default="transcriptions")
parser.add_argument("--id", default="Editor")
args = parser.parse_args()

app = Flask(__name__)
app.config['SECRET_KEY'] = 'secret!'
# socketio = SocketIO(async_mode='threading')
socketio = SocketIO()
rdb = redis.Redis(host=args.redisHost, port=6379, db=0)
p = rdb.pubsub()
thread = None


@app.route('/')
def index():
  return render_template('index.html', async_mode=socketio.async_mode)


@socketio.on('connect')
def connect():
  print('%s socket connected!' % args.id)
  # socketio.emit('message', 'acking your connect')
  global thread
  if thread is None:
    print('starting new background task')
    p.subscribe(args.redisChannel)
    thread = socketio.start_background_task(target=start_listening)


def start_listening():
  for msg in p.listen():
    # print('Read pubsub msg:' + str(msg))
    if 'subscribe' not in msg['type']:
      payload = msg['data'].decode('utf-8')
      socketio.emit('pubsubmsg', payload)
    socketio.sleep(0.2)


@socketio.on('disconnect')
def disconnect():
  print('%s socket disconnected!' % args.id)


@socketio.on('message')
def handle_message(message):
  print('Ignoring received string message: ' + message)


if __name__ == '__main__':
  socketio.init_app(app)
  socketio.run(app, host=args.host, port=args.port)
