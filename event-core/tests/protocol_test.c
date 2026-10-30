#include <assert.h>
#include <stdio.h>
#include <string.h>

/*
 * Include the implementation in this translation unit so the protocol's
 * private validators and state transitions can be tested without exporting
 * internal symbols as a public C API. PP_IPC_TESTING replaces only kevent
 * registration; socket and event-loop behavior remain integration concerns.
 */
#define main event_core_service_main
#include "../src/main.c"
#undef main

static void
reset_clients(void)
{
    for (size_t i = 0; i < MAX_CLIENTS; i++) {
        memset(&clients[i], 0, sizeof(clients[i]));
        clients[i].fd = -1;
    }
    notification_sequence = 0;
}

static void
expect_response(struct client *client, const char *request,
    const char *expected)
{
    char line[INPUT_CAP];
    size_t length = strlen(request);
    assert(length < sizeof(line));
    memcpy(line, request, length + 1);
    assert(handle_request(client, line) == 0);
    assert(strcmp(client->output, expected) == 0);
}

static void
test_identifier_validation(void)
{
    assert(valid_request_id("r1"));
    assert(valid_request_id("req_01-abc"));
    assert(!valid_request_id(""));
    assert(!valid_request_id("bad.id"));
    assert(!valid_request_id("bad id"));

    assert(valid_event_id("550e8400-e29b-41d4-a716-446655440000"));
    assert(!valid_event_id("550e8400-e29b-01d4-a716-446655440000"));
    assert(!valid_event_id("550e8400-e29b-41d4-7116-446655440000"));
    assert(!valid_event_id("not-a-uuid"));

    assert(valid_event_type("portal.content.updated"));
    assert(valid_event_type("a"));
    assert(!valid_event_type("Portal.Content"));
    assert(!valid_event_type("1portal"));
    assert(!valid_event_type("portal/type"));
    assert(!valid_event_type(""));
}

static void
test_request_responses(void)
{
    struct client client;
    memset(&client, 0, sizeof(client));
    client.fd = 10;

    expect_response(&client, "v2 req1 PING", "v2 req1 OK PONG\n");
    expect_response(&client, "v2 req2 STATUS", "v2 req2 OK RUNNING\n");
    expect_response(&client, "v2 req3 FUTURE", "v2 req3 ERROR UNKNOWN_COMMAND\n");
    expect_response(&client, "v1 req4 PING", "v2 0 ERROR BAD_REQUEST\n");
    expect_response(&client, "v2 req5 PING extra", "v2 0 ERROR BAD_REQUEST\n");
    expect_response(&client, "v2 req6 EVENT invalid portal.event", "v2 0 ERROR BAD_REQUEST\n");
    expect_response(&client,
        "v2 req7 EVENT 550e8400-e29b-41d4-a716-446655440000 portal.content.updated",
        "v2 req7 OK ACCEPTED\n");
}

static void
test_subscriber_ack_and_fifo(void)
{
    reset_clients();

    struct client subscriber;
    memset(&subscriber, 0, sizeof(subscriber));
    subscriber.fd = 20;
    clients[0] = subscriber;

    expect_response(&clients[0], "v2 sub1 SUBSCRIBE", "v2 sub1 OK SUBSCRIBED\n");
    assert(clients[0].subscribed == 1);
    /* Model write_client() completing the SUBSCRIBED response. */
    clients[0].output_len = 0;
    clients[0].output_sent = 0;

    struct client publisher;
    memset(&publisher, 0, sizeof(publisher));
    publisher.fd = 21;
    expect_response(&publisher,
        "v2 pub1 PUBLISH 550e8400-e29b-41d4-a716-446655440000 portal.content.updated",
        "v2 pub1 OK PUBLISHED\n");
    assert(clients[0].awaiting_ack == 1);
    assert(strcmp(clients[0].pending_event_id, "550e8400-e29b-41d4-a716-446655440000") == 0);
    assert(strcmp(clients[0].pending_event_type, "portal.content.updated") == 0);
    assert(clients[0].output_is_push == 1);
    /* Model write_client() completing the push; ACK is still outstanding. */
    clients[0].output_len = 0;
    clients[0].output_sent = 0;
    clients[0].output_is_push = 0;

    broadcast_event("550e8400-e29b-41d4-a716-446655440001", "portal.content.updated");
    assert(clients[0].queue_count == 1);

    expect_response(&clients[0],
        "v2 ack0 ACK 550e8400-e29b-41d4-a716-446655440002 portal.content.updated",
        "v2 ack0 ERROR BAD_ACK\n");
    assert(clients[0].awaiting_ack == 1);

    expect_response(&clients[0],
        "v2 ack1 ACK 550e8400-e29b-41d4-a716-446655440000 portal.content.updated",
        "v2 ack1 OK ACKNOWLEDGED\n");
    assert(clients[0].awaiting_ack == 0);
    assert(clients[0].pending_event_id[0] == '\0');
    /* Model the ACK response being fully written before the next push. */
    clients[0].output_len = 0;
    clients[0].output_sent = 0;

    assert(queue_next_event(&clients[0]) == 0);
    assert(clients[0].queue_count == 0);
    assert(clients[0].awaiting_ack == 1);
    assert(clients[0].output_is_push == 1);
    assert(strstr(clients[0].output,
        "550e8400-e29b-41d4-a716-446655440001 portal.content.updated") != NULL);
}

static void
test_subscriber_queue_is_bounded(void)
{
    reset_clients();
    clients[0].fd = 30;
    clients[0].subscribed = 1;
    clients[0].awaiting_ack = 1;
    strcpy(clients[0].pending_event_id, "550e8400-e29b-41d4-a716-446655440000");
    strcpy(clients[0].pending_event_type, "portal.content.updated");

    for (size_t i = 0; i < EVENT_QUEUE_CAP + 5; i++) {
        broadcast_event("550e8400-e29b-41d4-a716-446655440001",
            "portal.content.updated");
    }
    assert(clients[0].queue_count == EVENT_QUEUE_CAP);
    assert(clients[0].queue_head == 0);
}

int
main(void)
{
    test_identifier_validation();
    test_request_responses();
    test_subscriber_ack_and_fifo();
    test_subscriber_queue_is_bounded();
    puts("C event-core protocol tests passed");
    return 0;
}
