#!/bin/sh
set -eu

: "${IMAGE_PATH:?set IMAGE_PATH to a local image}"

curl --fail-with-body \
  --request POST \
  --form 'submission_id=upload-42' \
  --form 'appointment_id=appt-17' \
  --form 'caption=Photo for my scheduled visit' \
  --form "image=@${IMAGE_PATH}" \
  http://localhost:8080/screen
