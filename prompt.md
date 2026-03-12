
Act as a Senior Go Backend Engineer. Please provide a complete project structure and code for a professional HTTP server in Go with the following requirements:

Architecture: Use a schema-first approach. Generate server stubs using oapi-codegen for the chi router.

API Specification: Create a sample openapi.yaml (v3.0) for a 'Task Manager' with GET and POST endpoints. Include a /swagger endpoint that serves the Swagger UI using go:embed to host the static assets.

Routing & Middleware: Use go-chi/chi/v5 with standard middleware: Logger, Recoverer, and Timeout.

Database: Implement a persistence layer using SQLite (via modernc.org/sqlite). Use a clean separation between the database repository and the HTTP handlers.

## Professional Standards:

Include a Makefile for generating code and running the server.

Implement graceful shutdown for the HTTP server.

Use structured logging and proper error handling with custom JSON error responses.

Organize the project using the Standard Go Project Layout (e.g., /cmd, /lib/handler, /lib/repository).

## The HTTP endpoints

Create the OpenAPI 3.0 specifications from this and the code for these endpoints.

Create integration tests for all of these endpoints.

Each user can register with /api/v1/self-register provided that they have a valid registration_key.
Registered users will be tracked in a table in the database.  The table will be registered_users.
It should have a unique primary key called user_id.
POST /api/v1/self-register
with JSON body of 
```
    { 
        "host_name": "name of host",
        "registration_key": "UUID-KEY",
        "host_url: "http://host:port"
    }
```
With a 200 success return of
```
    {
        "user_id": ID,
        "per_user_api_key": "uak-UUID-KEY"
    }
```
And with errors with other status values.


Each user can create, update, delete and list the events that they have created.   This are tracked in a 2nd 
table in the database called user_events.
POST /api/v1/create-timed-event
```
    {
        "event_name": "A user supplied event name",
        "per_user_api_key": "uak-UUID-KEY",
        "cron_spec": "CRON specification for frequency of event - this or human_spec required.",
        "human_spec": "text specification for frequency of event - this or cron_spec required.",
        "body_tempalte": "{ go text template of the body that will be posted }"
    }
```
Returns a JSON of
```
    {
        "event_id": "evid-UUID"
    }
```

Update an event.  All of the JSON fields are optional except event_id and per_user_api_key.
Only the specified fields will be updated.
POST /api/v1/update-timed-event
```
    {
        "event_id": "evid-UUID",
        "event_name": "A user supplied event name",
        "per_user_api_key": "uak-UUID-KEY",
        "cron_spec": "CRON specification for frequency of event - this or human_spec required.",
        "human_spec": "text specification for frequency of event - this or cron_spec required.",
        "body_tempalte": "{ go text template of the body that will be posted }"
    }
```

POST /api/v1/delete-timed-event
```
    {
        "event_id": "evid-UUID"
    }
```

List out a users events.   This is for a single user.
POST /api/v1/list-timed-event to list all of the events of this user.
```
    {
        "per_user_api_key": "uak-UUID-KEY",
    }
```
Returns
```
    {
        "status": "sucess",
        "data": [
            { ... data ... }
        ]
    }
```

Search for events by name.
POST /api/v1/search-timed-event to search a users events.
```
    {
        "per_user_api_key": "uak-UUID-KEY",
        "event_name": "the name of an event - or"
    }
```
Returns
```
    {
        "status": "sucess",
        "data": [
            { ... data ... }
        ]
    }
```
