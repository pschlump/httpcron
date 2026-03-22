
# HTTPCron a go server that implements a `cron` like service for HTTP requests.

The `cron` scheduler uses an extended format with seconds.  There
is a 2nd format that is an English based specification for scheduling
actions.   To be able to run CLI commands on the target system a
HTTP client is provided.   The HTTP client validates that commands
are valid using an public/private key validated JWT authentication
token.


