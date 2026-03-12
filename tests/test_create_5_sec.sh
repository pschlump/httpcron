#!/bin/bash

mkdir -p ./out

# 1. Register 

#wget -o ./out/self-register.err -O ./out/self-register.json \
#  --method=POST \
#  --header="Content-Type: application/json" \
#  --body-data='{"host_url":"http://127.0.0.1:8080","host_name":"Philip test 1","registration_key":"'"$REGISTRATION_KEY"'"}' \
#  'http://127.0.0.1:9118/api/v1/self-register'

#echo ""
#cat ./out/self-register.err
#echo ""
#cat ./out/self-register.json | jq .
#echo ""


if grep "200 OK" ./out/self-register.err >/dev/null ; then
	echo "OK Registered"
else
	echo "Failed to regiter...."
	echo ""
	cat ./out/self-register.err
	echo ""
	exit 1
fi

# Example Output:	
# {"user_id":"f777bf3e-18e5-4500-8dea-24ef56f1a482","per_user_api_key":"uak-ef57e7a8-de5d-438b-ae94-c06cec55e06e"}

user_id=$( jq -r .user_id <out/self-register.json )
per_user_api_key=$( jq -r .per_user_api_key <out/self-register.json )

# echo "user_id=$user_id, per_user_api_key=$per_user_api_key"




# 2. Create event every 5 seconds
# ------------------------------------------------------------------------------------------------------------------------

wget -o ./out/every-5-sec.err -O ./out/every-5-sec.json \
  --method=POST \
  --header="Content-Type: application/json" \
  --body-data='{
		"event_name":"5 second test event",
		"cron_spec":"* */5 * * * *",
		"per_user_api_key":"'"$per_user_api_key"'",
		"url":"http://127.0.0.1:8080/index.html",
		"http_method":"GET"}' \
  'http://127.0.0.1:9118/api/v1/create-timed-event'

echo ""
cat ./out/every-5-sec.err
echo ""
cat ./out/every-5-sec.json | jq .
echo ""

