package loraserver

import (
	"errors"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	"github.com/jmoiron/sqlx"
)

// SQLNodeManager implements a SQL version of the NodeManager.
type SQLNodeManager struct {
	DB *sqlx.DB
}

// NewNodeManager creates a new SQLNodeManager.
func NewNodeManager(db *sqlx.DB) (NodeManager, error) {
	return &SQLNodeManager{db}, nil
}

// create creates the given Node.
func (s *SQLNodeManager) create(n models.Node) error {
	_, err := s.DB.Exec("insert into node (dev_eui, app_eui, app_key) values ($1, $2, $3)",
		n.DevEUI[:],
		n.AppEUI[:],
		n.AppKey[:],
	)
	if err == nil {
		log.WithField("dev_eui", n.DevEUI).Info("node created")
	}
	return err
}

// update updates the given Node.
func (s *SQLNodeManager) update(n models.Node) error {
	res, err := s.DB.Exec("update node set app_eui = $1, app_key = $2, used_dev_nonces = $3 where dev_eui = $4",
		n.AppEUI[:],
		n.AppKey[:],
		n.UsedDevNonces,
		n.DevEUI[:],
	)
	if err != nil {
		return err
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return errors.New("DevEUI did not match any rows")
	}
	log.WithField("dev_eui", n.DevEUI).Info("node updated")
	return nil
}

// delete deletes the Node matching the given DevEUI.
func (s *SQLNodeManager) delete(devEUI lorawan.EUI64) error {
	res, err := s.DB.Exec("delete from node where dev_eui = $1",
		devEUI[:],
	)
	if err != nil {
		return err
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return errors.New("DevEUI did not match any rows")
	}
	log.WithField("dev_eui", devEUI).Info("node deleted")
	return nil
}

// get returns the Node for the given DevEUI.
func (s *SQLNodeManager) get(devEUI lorawan.EUI64) (models.Node, error) {
	var node models.Node
	return node, s.DB.Get(&node, "select * from node where dev_eui = $1", devEUI[:])
}

// getList returns a slice of nodes, sorted by DevEUI.
func (s *SQLNodeManager) getList(limit, offset int) ([]models.Node, error) {
	var nodes []models.Node
	return nodes, s.DB.Select(&nodes, "select * from node order by dev_eui limit $1 offset $2", limit, offset)
}
