package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func runCommandTaskExecutor(ctx context.Context, db *sql.DB) {
	workerID := fmt.Sprintf("root-executor-%d-%d", os.Getpid(), time.Now().UnixNano())
	socketPath := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_EVENT_SOCKET"))
	if socketPath == "" {
		socketPath = defaultSocket
	}
	wake := make(chan struct{}, 256)
	ipc := IPCClient{SocketPath: socketPath, Timeout: 750 * time.Millisecond}
	go ipc.Subscribe(ctx, wake)
	_, err := db.ExecContext(ctx, `INSERT INTO portal.command_executor_heartbeat(singleton,worker_id,status,started_at,last_seen_at,last_error) VALUES(TRUE,$1,'ready',now(),now(),'') ON CONFLICT (singleton) DO UPDATE SET worker_id=EXCLUDED.worker_id,status='ready',started_at=now(),last_seen_at=now(),last_error=''`, workerID)
	if err != nil {
		log.Printf("command_executor_heartbeat_start_failed error=%v", err)
	}
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				qctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				_, e := db.ExecContext(qctx, `UPDATE portal.command_executor_heartbeat SET status='ready',last_seen_at=now(),last_error='' WHERE singleton=TRUE AND worker_id=$1`, workerID)
				cancel()
				if e != nil {
					log.Printf("command_executor_heartbeat_failed error=%v", e)
				}
			}
		}
	}()
	executor := privilegedTaskExecutor{DB: db, WorkerID: workerID, PollInterval: commandTaskPoll, Wake: wake}
	executor.Run(ctx)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = db.ExecContext(shutdownCtx, `UPDATE portal.command_executor_heartbeat SET status='stopping',last_seen_at=now() WHERE singleton=TRUE AND worker_id=$1`, workerID)
	executor.CancelRunning(shutdownCtx)
	<-heartbeatDone
	_, _ = db.ExecContext(shutdownCtx, `UPDATE portal.command_executor_heartbeat SET status='offline',last_seen_at=now() WHERE singleton=TRUE AND worker_id=$1`, workerID)
}
