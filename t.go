func (db *Database) StoreMessage(roomName string, msg internal.Message) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Just Insert. No Delete.
	query := `INSERT INTO messages (room_id, id, timestamp, data) VALUES ($1, $2, $3, $4)`
	_, err = db.Conn.Exec(query, roomName, msg.ID, msg.Timestamp, msgJSON)
	return err
}

// PruneMessages handles cleanup in the background (Heavy)
func (db *Database) PruneMessages(keep int) error {
	// 1. Get list of active rooms (distinct room_ids)
	rows, err := db.Conn.Query(`SELECT DISTINCT room_id FROM messages`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var rooms []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err == nil {
			rooms = append(rooms, r)
		}
	}

	// 2. Prune each room independently
	// Note: In a massive production app, you'd use a more complex single query or partition deletion,
	// but for this scale, iterating rooms is perfectly fine and safe.
	stmt := `DELETE FROM messages 
	         WHERE room_id = $1 AND id NOT IN (
	             SELECT id FROM messages 
	             WHERE room_id = $1 
	             ORDER BY timestamp DESC 
	             LIMIT $2
	         )`

	for _, room := range rooms {
		_, err := db.Conn.Exec(stmt, room, keep)
		if err != nil {
			fmt.Printf("Failed to prune room %s: %v\n", room, err)
		}
	}
	return nil
}

