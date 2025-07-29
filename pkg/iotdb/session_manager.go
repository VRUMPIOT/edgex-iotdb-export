package iotdb

import (
	"fmt"
	"sync"
	"time"

	"github.com/apache/iotdb-client-go/v2/client"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/clients/logger"

	"app-iotdb-export/pkg/dtos"
)

type SessionManager struct {
	mutex                 sync.RWMutex
	lc                    logger.LoggingClient
	session               *client.Session
	config                client.Config
	enableRPCCompression  bool
	connectionTimeoutInMs int
	lastUsed              time.Time
}

func NewSessionManager(lc logger.LoggingClient, config client.Config,
	enableRPCCompression bool, connectionTimeoutInMs int) *SessionManager {
	return &SessionManager{
		lc:                    lc,
		config:                config,
		enableRPCCompression:  enableRPCCompression,
		connectionTimeoutInMs: connectionTimeoutInMs,
	}
}

// SendRecords handles the complete record insertion with detailed logging
func (sm *SessionManager) SendRecords(data dtos.Readings) error {
	sm.lc.Debugf("Beginning SendRecords for %d devices", len(data.DeviceIds))
	defer func() {
		sm.lc.Debugf("SendRecords completed")
	}()

	session, err := sm.GetSession()
	if err != nil {
		return fmt.Errorf("session acquisition failed: %w", err)
	}

	sm.lc.Tracef("Attempting to insert %d records with session %d",
		len(data.DeviceIds), sm.session.GetSessionId())

	status, err := (*session).InsertRecords(
		data.DeviceIds,
		data.Measurements,
		data.DataTypes,
		data.Values,
		data.Timestamps,
	)

	if err != nil {
		sm.invalidateSession()
		return fmt.Errorf("resetting session, insert operation failed: %w", err)
	}

	if status.Code != 200 && status.Code != 400 {
		return fmt.Errorf("insert failed with status code: %d", status.Code)
	}

	sm.lc.Infof("Successfully inserted %d records (session %d)",
		len(data.DeviceIds), sm.session.GetSessionId())
	return nil
}

func (sm *SessionManager) GetSession() (*client.Session, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	timeoutDuration := time.Duration(sm.connectionTimeoutInMs) * time.Millisecond

	// Check for existing valid session
	if sm.session != nil {
		sm.lastUsed = time.Now()
		sm.lc.Tracef("Reusing existing session %d (last used %v ago)",
			sm.session.GetSessionId(), time.Since(sm.lastUsed))
		return sm.session, nil
	}

	// Clean up existing session if invalid
	sm.invalidateSession()

	newSession := client.NewSession(&sm.config)

	// Open connection with timeout
	sm.lc.Debugf("Opening connection (timeout: %v, compression: %v)",
		timeoutDuration, sm.enableRPCCompression)

	if err := newSession.Open(sm.enableRPCCompression, sm.connectionTimeoutInMs); err != nil {
		return nil, fmt.Errorf("failed to open session: %w", err)
	}

	sm.session = &newSession
	sm.lastUsed = time.Now()
	sm.lc.Infof("Created new session %d", sm.session.GetSessionId())

	return sm.session, nil
}

func (sm *SessionManager) invalidateSession() {
	if sm.session != nil {
		sm.lc.Debugf("Invalidating session %d", sm.session.GetSessionId())
		if err := (*sm.session).Close(); err != nil {
			sm.lc.Warnf("Error closing session %d: %v", sm.session.GetSessionId(), err)
		} else {
			sm.lc.Debugf("Successfully closed session %d", sm.session.GetSessionId())
		}
		sm.session = nil
	}
}

func (sm *SessionManager) Close() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.lc.Info("Closing SessionManager and cleaning up resources")
	sm.invalidateSession()
}
