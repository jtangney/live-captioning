import argparse
import asyncio
import base64

import redis
import websockets


async def ingest(websocket, path):
  async for message in websocket:
    print('got data')
    await enqueue(message)
    # encoded = base64.b64encode(message)
    # rdb.lpush('liveq', encoded)


async def enqueue(message):
  encoded = base64.b64encode(message)
  rdb.lpush('liveq', encoded)


parser = argparse.ArgumentParser()
parser.add_argument("--redisHost")
args = parser.parse_args()
print(args.redisHost)

rdb = redis.Redis(host=args.redisHost, port=6379, db=0)
start_server = websockets.serve(ingest, "0.0.0.0", 8080)
asyncio.get_event_loop().run_until_complete(start_server)
asyncio.get_event_loop().run_forever()
