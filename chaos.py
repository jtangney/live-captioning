import argparse
import random
import subprocess
import sys
import time

parser = argparse.ArgumentParser()
parser.add_argument('--applabel', default=None)
parser.add_argument('--count', default=1, type=int)
parser.add_argument('--iterations', default=1, type=int)
parser.add_argument('--delay', default=30, type=int)
args = parser.parse_args()


def kill_pod(app_label):
  get_pods_command = 'kubectl get pods -o jsonpath={.items[*].metadata.name}'
  if app_label:
    get_pods_command += ' -l=app=%s' % app_label
  print(get_pods_command)
  elems = get_pods_command.split(' ')
  all_pods = subprocess.check_output(elems).decode('utf-8')
  if not all_pods:
    print('ERROR: no pods found matching command!')
    sys.exit(1)
  print(all_pods)

  pods = all_pods.split(' ')
  victims = random.sample(pods, min(args.count, len(pods)))
  print('killing %d pod(s): %s' % (args.count, victims))

  for victim in victims:
    kill_pod_command = 'kubectl delete pod %s --force --grace-period=0' % victim
    elems = kill_pod_command.split(' ')
    result = subprocess.check_output(elems)
    print('%s at %s' % (result.decode('utf-8'), time.strftime("%H:%M:%S", time.gmtime())))


kill_pod(args.applabel)
for i in range(args.iterations - 1):
  time.sleep(args.delay)
  kill_pod(args.applabel)
