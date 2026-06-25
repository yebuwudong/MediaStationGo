package service

import "time"

func (d *DownloadService) currentTime() time.Time {
	if d != nil && d.now != nil {
		return d.now()
	}
	return time.Now()
}

func (d *DownloadService) recordLiveTorrentSnapshot(live []QBitTorrent) {
	if d == nil {
		return
	}
	snapshot := cloneQBitTorrentSlice(live)
	d.mu.Lock()
	d.liveTorrents = snapshot
	d.liveTorrentsAt = d.currentTime()
	d.mu.Unlock()
}

func (d *DownloadService) LiveTorrentSnapshot(maxAge time.Duration) []QBitTorrent {
	if d == nil {
		return nil
	}
	now := d.currentTime()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.liveTorrentsAt.IsZero() {
		return nil
	}
	if maxAge > 0 && now.Sub(d.liveTorrentsAt) > maxAge {
		return nil
	}
	return cloneQBitTorrentSlice(d.liveTorrents)
}

func cloneQBitTorrentSlice(in []QBitTorrent) []QBitTorrent {
	if len(in) == 0 {
		return nil
	}
	return append([]QBitTorrent(nil), in...)
}
