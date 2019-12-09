FROM python:3.7-slim

WORKDIR /app
ADD . /app

RUN pip install -r requirements.txt
ENTRYPOINT [ "python", "-u", "/app/editor.py" ]