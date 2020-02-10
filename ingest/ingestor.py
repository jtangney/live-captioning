import argparse
import base64
import queue

import eventlet
import redis
from flask import Flask
from flask_socketio import SocketIO

# for socketio background tasks
eventlet.monkey_patch()

parser = argparse.ArgumentParser()
parser.add_argument("--host", default="localhost")
parser.add_argument("--port", default=8080)
parser.add_argument("--redisHost", required=True)
parser.add_argument("--redisQueue", default="liveq")
parser.add_argument("--id", default="Ingest")
args = parser.parse_args()

app = Flask(__name__)
socketio = SocketIO()
rdb = redis.Redis(host=args.redisHost, port=6379, db=0,
                  health_check_interval=2, socket_timeout=3)
buff = queue.Queue()


@socketio.on('connect')
def connect():
  print('%s socket connected!' % args.id)
  socketio.emit('pod_id', args.id)


@socketio.on('disconnect')
def disconnect():
  print('%s socket disconnected!' % args.id)


@socketio.on('data')
def handle_data(data):
  """Stores the received audio data in a local buffer"""
  encoded = base64.b64encode(data)
  buff.put(encoded, block=False)


def _enqueue_audio(redis_queue):
  """Blocking-reads data from the buffer and adds to Redis queue"""
  while True:
    try:
      chunk = buff.get(block=True)
      val = rdb.lpush(redis_queue, chunk)
      # debugging; under normal circumstances audio should not be accumulating
      if val > 5:
        print('Ingested audio queue length: %d' % val)
    except redis.exceptions.RedisError as err:
      print('Error pushing into Redis queue: %s' % err)


@app.route('/')
def hello_world():
  return 'Hello from ' + args.id


if __name__ == '__main__':
  socketio.init_app(app)
  socketio.start_background_task(_enqueue_audio, args.redisQueue)
  socketio.run(app, host=args.host, port=args.port)

