
```

/api/v1/self-register 
    In Registration Token
    Out Per User Auth Key
    Out Per User user_id value

/api/v1/create-event
/api/v1/list-event
/api/v1/delete-event
/api/v1/update-event
/api/v1/search-event
    event_id
    dest_url
    last_call
    call_freq 


create_event:
{
    "user_id": "User id from /api/v1/self-register"
    "per-user-auth-key": "per key from /api/v1/self-register"
    "name": "A unique to this registered user name"
    "cron_spec": "cron scheduling string"
    "human_schedule": "human string that will be parsed into cron schedule"
    "callback_url": "http://.../"
    "callback_data": "go text tempate for rcallback JSON data"
    "on": {
        "status": 200
        "status": 401
        "timeout":  backoff 
    }

}

 
