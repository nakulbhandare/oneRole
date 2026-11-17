#!/bin/bash
# Combined entrypoint for Railway free plan
echo "Starting OneRole API..."
go run . & 
API_PID=$!
echo "Starting OneRole UI..."
cd oneRole_UI && npm install && npm start &
UI_PID=$!
wait $API_PID $UI_PID
