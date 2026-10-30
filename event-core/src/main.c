#include <sys/types.h>
#include <sys/event.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <sys/stat.h>

#include <errno.h>
#include <fcntl.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

#include "event_protocol.h"

#define DEFAULT_SOCKET_PATH "/var/run/personal-portal/event.sock"
#define MAX_CLIENTS PP_IPC_MAX_CLIENTS
#define INPUT_CAP (PP_IPC_REQUEST_MAX_BYTES + 2)
#define OUTPUT_CAP 256
#define EVENT_BATCH 32
#define EVENT_QUEUE_CAP 32
#define CLIENT_REQUEST_TIMEOUT_SECONDS 5
#define CLIENT_ACK_TIMEOUT_SECONDS 5

struct queued_event {
    char event_id[37];
    char event_type[101];
};

struct client {
    int fd;
    char input[INPUT_CAP];
    size_t input_len;
    char output[OUTPUT_CAP];
    size_t output_len;
    size_t output_sent;
    int subscribed;
    int awaiting_ack;
    int output_is_push;

    char pending_event_id[37];
    char pending_event_type[101];
    struct queued_event queue[EVENT_QUEUE_CAP];
    size_t queue_head;
    size_t queue_count;
    time_t last_activity;
    time_t ack_started_at;
};

static struct client clients[MAX_CLIENTS];
static int kq = -1;
static int listener = -1;
static int stopping;
static int socket_path_created;
static unsigned long notification_sequence;
static const char *socket_path = DEFAULT_SOCKET_PATH;

static int
set_nonblocking(int fd)
{
    int flags = fcntl(fd, F_GETFL, 0);
    return flags == -1 ? -1 : fcntl(fd, F_SETFL, flags | O_NONBLOCK);
}

static int
change_event(uintptr_t ident, int16_t filter, uint16_t flags, void *udata)
{
#ifdef PP_IPC_TESTING
    /* Protocol tests exercise state transitions without opening a kqueue. */
    (void)ident;
    (void)filter;
    (void)flags;
    (void)udata;
    return 0;
#else
    struct kevent event;
    EV_SET(&event, ident, filter, flags, 0, 0, udata);
    return kevent(kq, &event, 1, NULL, 0, NULL);
#endif
}

static void
close_client(struct client *client)
{
    if (client->fd >= 0) {
        if (kq >= 0) {
            (void)change_event((uintptr_t)client->fd, EVFILT_READ,
                EV_DELETE, client);
            (void)change_event((uintptr_t)client->fd, EVFILT_WRITE,
                EV_DELETE, client);
        }
        (void)close(client->fd);
    }
    memset(client, 0, sizeof(*client));
    client->fd = -1;
}

static struct client *
find_free_client(void)
{
    for (size_t i = 0; i < MAX_CLIENTS; i++) {
        if (clients[i].fd == -1)
            return &clients[i];
    }
    return NULL;
}

static int
queue_response(struct client *client, const char *request_id,
    const char *status, const char *payload)
{
    int length = snprintf(client->output, sizeof(client->output),
        PP_IPC_PROTOCOL_VERSION " %s %s %s\n", request_id, status, payload);

    if (length < 0 || (size_t)length >= sizeof(client->output))
        return -1;

    client->output_len = (size_t)length;
    client->output_sent = 0;
    client->output_is_push = 0;

    if (change_event((uintptr_t)client->fd, EVFILT_READ, EV_DELETE,
        client) == -1 && errno != ENOENT)
        return -1;

    return change_event((uintptr_t)client->fd, EVFILT_WRITE,
        EV_ADD | EV_ENABLE, client);
}

static int
valid_request_id(const char *value)
{
    if (*value == '\0')
        return 0;
    for (const unsigned char *p = (const unsigned char *)value; *p; p++) {
        if (!((*p >= '0' && *p <= '9') ||
            (*p >= 'A' && *p <= 'Z') ||
            (*p >= 'a' && *p <= 'z') || *p == '-' || *p == '_'))
            return 0;
    }
    return 1;
}

static int
valid_event_id(const char *value)
{
    if (strlen(value) != 36)
        return 0;
    for (size_t i = 0; i < 36; i++) {
        if (i == 8 || i == 13 || i == 18 || i == 23) {
            if (value[i] != '-')
                return 0;
        } else {
            unsigned char ch = (unsigned char)value[i];
            if (!((ch >= '0' && ch <= '9') ||
                (ch >= 'a' && ch <= 'f') ||
                (ch >= 'A' && ch <= 'F')))
                return 0;
        }
    }
    if (value[14] < '1' || value[14] > '8')
        return 0;
    return value[19] == '8' || value[19] == '9' ||
        value[19] == 'a' || value[19] == 'A' ||
        value[19] == 'b' || value[19] == 'B';
}

static int
valid_event_type(const char *value)
{
    size_t length = strlen(value);
    if (length == 0 || length > 100 ||
        value[0] < 'a' || value[0] > 'z')
        return 0;
    for (size_t i = 1; i < length; i++) {
        unsigned char ch = (unsigned char)value[i];
        if (!((ch >= 'a' && ch <= 'z') ||
            (ch >= '0' && ch <= '9') || ch == '.' ||
            ch == '_' || ch == '-'))
            return 0;
    }
    return 1;
}

static int
queue_event_push(struct client *client, const char *event_id,
    const char *event_type)
{
    int length = snprintf(client->output, sizeof(client->output),
        PP_IPC_PROTOCOL_VERSION " push%lu EVENT %s %s\n", ++notification_sequence,
        event_id, event_type);
    if (length < 0 || (size_t)length >= sizeof(client->output))
        return -1;
    client->output_len = (size_t)length;
    client->output_sent = 0;
    client->output_is_push = 1;
    client->awaiting_ack = 1;
    client->ack_started_at = time(NULL);
    memcpy(client->pending_event_id, event_id, strlen(event_id) + 1);
    memcpy(client->pending_event_type, event_type, strlen(event_type) + 1);
    printf("event_core_push_queued event_id=%s event_type=%s queue_depth=%zu\n",
        event_id, event_type, client->queue_count);
    return change_event((uintptr_t)client->fd, EVFILT_WRITE,
        EV_ADD | EV_ENABLE, client);
}

static int
queue_next_event(struct client *client)
{
    if (client->queue_count == 0 || client->awaiting_ack)
        return 0;
    struct queued_event event = client->queue[client->queue_head];
    client->queue_head = (client->queue_head + 1) % EVENT_QUEUE_CAP;
    client->queue_count--;
    return queue_event_push(client, event.event_id, event.event_type);
}

static void
broadcast_event(const char *event_id, const char *event_type)
{
    for (size_t i = 0; i < MAX_CLIENTS; i++) {
        struct client *client = &clients[i];
        if (client->fd < 0 || !client->subscribed)
            continue;
        if (!client->awaiting_ack && client->output_len == 0) {
            if (queue_event_push(client, event_id, event_type) == -1)
                close_client(client);
            continue;
        }
        if (client->queue_count >= EVENT_QUEUE_CAP) {
            printf("event_core_push_queue_overflow event_id=%s event_type=%s capacity=%d\n",
                event_id, event_type, EVENT_QUEUE_CAP);
            continue;
        }
        size_t tail = (client->queue_head + client->queue_count) % EVENT_QUEUE_CAP;
        memcpy(client->queue[tail].event_id, event_id, strlen(event_id) + 1);
        memcpy(client->queue[tail].event_type, event_type, strlen(event_type) + 1);
        client->queue_count++;
        printf("event_core_push_buffered event_id=%s event_type=%s queue_depth=%zu\n",
            event_id, event_type, client->queue_count);
    }
}

static int
handle_request(struct client *client, char *line)
{
    char version[8] = {0};
    char request_id[32] = {0};
    char command[32] = {0};
    char event_id[37] = {0};
    char event_type[101] = {0};
    char extra[2] = {0};
    int fields = sscanf(line, "%7s %31s %31s %36s %100s %1s",
        version, request_id, command, event_id, event_type, extra);

    if (strcmp(version, PP_IPC_PROTOCOL_VERSION) != 0 || !valid_request_id(request_id))
        return queue_response(client, "0", "ERROR", "BAD_REQUEST");

    if (strcmp(command, "PING") == 0 || strcmp(command, "STATUS") == 0) {
        if (fields != 3)
            return queue_response(client, "0", "ERROR", "BAD_REQUEST");
        if (strcmp(command, "PING") == 0)
            return queue_response(client, request_id, "OK", "PONG");
        return queue_response(client, request_id, "OK", "RUNNING");
    }

    if (strcmp(command, "SUBSCRIBE") == 0) {
        if (fields != 3)
            return queue_response(client, "0", "ERROR", "BAD_REQUEST");
        client->subscribed = 1;
        return queue_response(client, request_id, "OK", "SUBSCRIBED");
    }

    if (strcmp(command, "PUBLISH") == 0) {
        if (fields != 5 || !valid_event_id(event_id) ||
            !valid_event_type(event_type))
            return queue_response(client, "0", "ERROR", "BAD_REQUEST");
        broadcast_event(event_id, event_type);
        return queue_response(client, request_id, "OK", "PUBLISHED");
    }

    if (strcmp(command, "ACK") == 0) {
        if (fields != 5 || !valid_event_id(event_id) ||
            !valid_event_type(event_type))
            return queue_response(client, "0", "ERROR", "BAD_REQUEST");
        if (!client->subscribed || !client->awaiting_ack ||
            strcmp(client->pending_event_id, event_id) != 0 ||
            strcmp(client->pending_event_type, event_type) != 0) {
            printf("event_core_push_ack_rejected event_id=%s event_type=%s\n", event_id, event_type);
            return queue_response(client, request_id, "ERROR", "BAD_ACK");
        }
        client->awaiting_ack = 0;
        client->ack_started_at = 0;
        client->pending_event_id[0] = '\0';
        client->pending_event_type[0] = '\0';
        printf("event_core_push_acknowledged event_id=%s event_type=%s\n", event_id, event_type);
        /* ACK response completion drains the next queued push. */
        return queue_response(client, request_id, "OK", "ACKNOWLEDGED");
    }

    if (strcmp(command, "EVENT") == 0) {
        if (fields != 5 || !valid_event_id(event_id) ||
            !valid_event_type(event_type))
            return queue_response(client, "0", "ERROR", "BAD_REQUEST");
        /* ACCEPTED validates metadata only; it is not business completion. */
        return queue_response(client, request_id, "OK", "ACCEPTED");
    }

    if (fields != 3)
        return queue_response(client, "0", "ERROR", "BAD_REQUEST");
    return queue_response(client, request_id, "ERROR", "UNKNOWN_COMMAND");
}

static void
accept_clients(void)
{
    for (;;) {
        int fd = accept(listener, NULL, NULL);
        if (fd == -1) {
            if (errno == EINTR)
                continue;
            if (errno == EAGAIN || errno == EWOULDBLOCK)
                return;
            perror("accept");
            return;
        }

        if (set_nonblocking(fd) == -1) {
            close(fd);
            continue;
        }

        struct client *client = find_free_client();
        if (client == NULL) {
            close(fd);
            continue;
        }

        memset(client, 0, sizeof(*client));
        client->fd = fd;
        client->last_activity = time(NULL);
        if (change_event((uintptr_t)fd, EVFILT_READ,
            EV_ADD | EV_ENABLE, client) == -1) {
            perror("kevent add client");
            close_client(client);
        }
    }
}

static void
read_client(struct client *client)
{
    for (;;) {
        if (client->input_len >= sizeof(client->input) - 1) {
            if (queue_response(client, "0", "ERROR",
                "REQUEST_TOO_LARGE") == -1)
                close_client(client);
            return;
        }

        ssize_t count = read(client->fd,
            client->input + client->input_len,
            sizeof(client->input) - 1 - client->input_len);

        if (count == 0) {
            close_client(client);
            return;
        }
        if (count == -1) {
            if (errno == EINTR)
                continue;
            if (errno == EAGAIN || errno == EWOULDBLOCK)
                return;
            close_client(client);
            return;
        }

        client->last_activity = time(NULL);
        client->input_len += (size_t)count;
        client->input[client->input_len] = '\0';

        if (memchr(client->input, '\0', client->input_len) != NULL) {
            if (queue_response(client, "0", "ERROR", "BAD_REQUEST") == -1)
                close_client(client);
            return;
        }

        char *newline = memchr(client->input, '\n', client->input_len);
        if (newline != NULL) {
            *newline = '\0';
            if (handle_request(client, client->input) == -1) {
                close_client(client);
            } else {
                client->input_len = 0;
                client->input[0] = '\0';
            }
            return;
        }
    }
}

static void
write_client(struct client *client)
{
    while (client->output_sent < client->output_len) {
        ssize_t count = write(client->fd,
            client->output + client->output_sent,
            client->output_len - client->output_sent);

        if (count == -1) {
            if (errno == EINTR)
                continue;
            if (errno == EAGAIN || errno == EWOULDBLOCK)
                return;
            close_client(client);
            return;
        }
        if (count == 0) {
            close_client(client);
            return;
        }
        client->last_activity = time(NULL);
        client->output_sent += (size_t)count;
    }
    if (client->subscribed) {
        int was_push = client->output_is_push;
        client->output_len = 0;
        client->output_sent = 0;
        client->output_is_push = 0;
        if (change_event((uintptr_t)client->fd, EVFILT_WRITE,
            EV_DELETE, client) == -1 && errno != ENOENT) {
            close_client(client);
            return;
        }
        if (!was_push && !client->awaiting_ack && client->queue_count > 0) {

            if (queue_next_event(client) == -1)
                close_client(client);
            return;
        }

        if (change_event((uintptr_t)client->fd, EVFILT_READ,
            EV_ADD | EV_ENABLE, client) == -1)
            close_client(client);
        return;
    }
    close_client(client);
}

static void
expire_clients(void)
{
    time_t now = time(NULL);
    for (size_t i = 0; i < MAX_CLIENTS; i++) {
        struct client *client = &clients[i];
        if (client->fd < 0)
            continue;
        if (client->awaiting_ack && client->ack_started_at > 0 &&
            now - client->ack_started_at >= CLIENT_ACK_TIMEOUT_SECONDS) {
            printf("event_core_ack_timeout fd=%d event_id=%s event_type=%s timeout_seconds=%d\n",
                client->fd, client->pending_event_id, client->pending_event_type,
                CLIENT_ACK_TIMEOUT_SECONDS);
            close_client(client);
            continue;
        }
        if (!client->subscribed && client->last_activity > 0 &&
            now - client->last_activity >= CLIENT_REQUEST_TIMEOUT_SECONDS) {
            printf("event_core_client_timeout fd=%d timeout_seconds=%d\n",
                client->fd, CLIENT_REQUEST_TIMEOUT_SECONDS);
            close_client(client);
        }
    }
}

static int
prepare_socket_path(const struct sockaddr_un *address)
{
    struct stat path_stat;
    if (lstat(socket_path, &path_stat) == -1) {
        if (errno == ENOENT)
            return 0;
        return -1;
    }

    /* Never unlink regular files, symlinks, or another service's path. */
    if (!S_ISSOCK(path_stat.st_mode)) {
        errno = EEXIST;
        return -1;
    }

    int probe = socket(AF_UNIX, SOCK_STREAM, 0);
    if (probe == -1)
        return -1;
    int connect_result = connect(probe, (const struct sockaddr *)address,
        sizeof(*address));
    int connect_error = errno;
    (void)close(probe);

    if (connect_result == 0) {
        errno = EADDRINUSE;
        return -1;
    }
    if (connect_error == ENOENT)
        return 0;
    if (connect_error != ECONNREFUSED) {
        errno = connect_error;
        return -1;
    }

    /* A stale socket may be removed only by the same service identity. */
    if (path_stat.st_uid != geteuid()) {
        errno = EACCES;
        return -1;
    }
    return unlink(socket_path);
}

static int
start_listener(void)
{
    struct sockaddr_un address;

    if (strlen(socket_path) >= sizeof(address.sun_path)) {
        fprintf(stderr, "socket path is too long\n");
        errno = ENAMETOOLONG;
        return -1;
    }

    memset(&address, 0, sizeof(address));
    address.sun_family = AF_UNIX;
    memcpy(address.sun_path, socket_path, strlen(socket_path) + 1);
    if (prepare_socket_path(&address) == -1)
        return -1;

    listener = socket(AF_UNIX, SOCK_STREAM, 0);
    if (listener == -1)
        return -1;
    if (set_nonblocking(listener) == -1)
        return -1;

    if (bind(listener, (struct sockaddr *)&address, sizeof(address)) == -1)
        return -1;
    socket_path_created = 1;
    if (chmod(socket_path, S_IRUSR | S_IWUSR) == -1)
        return -1;
    if (listen(listener, 32) == -1)
        return -1;
    return 0;
}

static void
cleanup(void)
{
    for (size_t i = 0; i < MAX_CLIENTS; i++)
        close_client(&clients[i]);

    if (listener >= 0) {
        close(listener);
        listener = -1;
    }
    if (kq >= 0) {
        close(kq);
        kq = -1;
    }
    if (socket_path_created && socket_path != NULL) {
        (void)unlink(socket_path);
        socket_path_created = 0;
    }
}

int
main(int argc, char **argv)
{
    /* The service redirects stdout to a file; flush each log line immediately. */
    (void)setvbuf(stdout, NULL, _IOLBF, 0);

    if (argc == 2) {
        socket_path = argv[1];
    } else if (argc > 2) {
        fprintf(stderr, "usage: %s [socket-path]\n", argv[0]);
        return EXIT_FAILURE;
    }

    for (size_t i = 0; i < MAX_CLIENTS; i++)
        clients[i].fd = -1;

    if (start_listener() == -1) {
        perror("start listener");
        cleanup();
        return EXIT_FAILURE;
    }

    kq = kqueue();
    if (kq == -1) {
        perror("kqueue");
        cleanup();
        return EXIT_FAILURE;
    }

    if (change_event((uintptr_t)listener, EVFILT_READ,
        EV_ADD | EV_ENABLE, NULL) == -1) {
        perror("kevent add listener");
        cleanup();
        return EXIT_FAILURE;
    }

    (void)signal(SIGPIPE, SIG_IGN);
    (void)signal(SIGINT, SIG_IGN);
    (void)signal(SIGTERM, SIG_IGN);

    if (change_event(SIGINT, EVFILT_SIGNAL, EV_ADD | EV_ENABLE, NULL) == -1 ||
        change_event(SIGTERM, EVFILT_SIGNAL, EV_ADD | EV_ENABLE, NULL) == -1) {
        perror("kevent add signal");
        cleanup();
        return EXIT_FAILURE;
    }

    printf("event-core listening on %s\n", socket_path);
    fflush(stdout);

    while (!stopping) {
        struct kevent events[EVENT_BATCH];
        struct timespec wait_timeout = { .tv_sec = 1, .tv_nsec = 0 };
        int count = kevent(kq, NULL, 0, events, EVENT_BATCH, &wait_timeout);

        if (count == -1) {
            if (errno == EINTR)
                continue;
            perror("kevent wait");
            stopping = 1;
            break;
        }

        for (int i = 0; i < count; i++) {
            struct kevent *event = &events[i];

            if (event->filter == EVFILT_SIGNAL) {
                stopping = 1;
                break;
            }
            if ((int)event->ident == listener &&
                event->filter == EVFILT_READ) {
                accept_clients();
                continue;
            }

            struct client *client = event->udata;
            if (client == NULL || client->fd != (int)event->ident)
                continue;
            if (event->flags & EV_ERROR) {
                close_client(client);
                continue;
            }
            if (event->filter == EVFILT_READ)
                read_client(client);
            else if (event->filter == EVFILT_WRITE)
                write_client(client);
        }
        expire_clients();
    }

    cleanup();
    puts("event-core stopped");
    return EXIT_SUCCESS;
}
