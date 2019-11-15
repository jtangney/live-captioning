import argparse
import base64

import redis
from flask import Flask
from flask_socketio import SocketIO

parser = argparse.ArgumentParser()
parser.add_argument("--host", default="localhost")
parser.add_argument("--port", default=8080)
parser.add_argument("--redisHost", required=True)
parser.add_argument("--redisQueue", default="liveq")
parser.add_argument("--id", default="Ingest")
args = parser.parse_args()

app = Flask(__name__)
app.config['SECRET_KEY'] = 'secret!'
socketio = SocketIO()
rdb = redis.Redis(host=args.redisHost, port=6379, db=0)


@app.route('/')
def hello_world():
  return 'hello from ' + args.id


@socketio.on('connect')
def connect():
  print('%s socket connected!' % args.id)


@socketio.on('disconnect')
def connect():
  print('%s socket disconnected!' % args.id)


@socketio.on('message')
def handle_message(message):
  print('Ignoring received string message: ' + message)


@socketio.on('data')
def handle_data(data):
  # print('received data')
  encoded = base64.b64encode(data)
  rdb.lpush(args.redisQueue, encoded)


if __name__ == '__main__':
  socketio.init_app(app)
  socketio.run(app, host=args.host, port=args.port)
