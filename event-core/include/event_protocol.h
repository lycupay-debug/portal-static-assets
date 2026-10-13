#ifndef PERSONAL_PORTAL_EVENT_PROTOCOL_H
#define PERSONAL_PORTAL_EVENT_PROTOCOL_H

/*
 * Personal Portal local IPC protocol v1.
 *
 * Wire format (ASCII, one request/response per connection):
 *   request:  v1 <request_id> <command>\n
 *   response: v1 <request_id> <status> <payload>\n
 *
 * This header defines shared protocol constants only. Parsing and validation
 * remain in the C event core and the future Go client.
 */

#define PP_IPC_PROTOCOL_VERSION "v1"
#define PP_IPC_COMMAND_PING "PING"
#define PP_IPC_COMMAND_STATUS "STATUS"

#define PP_IPC_STATUS_OK "OK"
#define PP_IPC_STATUS_ERROR "ERROR"

#define PP_IPC_PAYLOAD_PONG "PONG"
#define PP_IPC_PAYLOAD_RUNNING "RUNNING"

#define PP_IPC_ERROR_BAD_REQUEST "BAD_REQUEST"
#define PP_IPC_ERROR_REQUEST_TOO_LARGE "REQUEST_TOO_LARGE"
#define PP_IPC_ERROR_UNKNOWN_COMMAND "UNKNOWN_COMMAND"

#define PP_IPC_REQUEST_MAX_BYTES 254
#define PP_IPC_REQUEST_ID_MAX_BYTES 31
#define PP_IPC_MAX_CLIENTS 64

#endif
