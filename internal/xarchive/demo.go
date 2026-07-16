package xarchive

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
	"unicode/utf16"
)

func (s *Store) ImportDemo() (ImportResult, error) {
	tempDir, err := os.MkdirTemp("", "vanish-x-demo-")
	if err != nil {
		return ImportResult{}, err
	}
	defer os.RemoveAll(tempDir)
	archivePath := filepath.Join(tempDir, "demo-x-archive.zip")
	if err := WriteDemoZIP(archivePath); err != nil {
		return ImportResult{}, err
	}
	return s.ImportZIP(archivePath, true)
}

// WriteDemoZIP creates a small, synthetic archive for tests and the in-app demo.
// It contains no user data and is never needed as a committed binary fixture.
func WriteDemoZIP(destination string) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	closeWithError := func(writeErr error) error {
		archiveErr := archive.Close()
		fileErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if archiveErr != nil {
			return archiveErr
		}
		return fileErr
	}

	account := []map[string]any{{"account": map[string]any{
		"accountId": "424242", "username": "vanish_demo", "accountDisplayName": "Vanish Demo",
	}}}
	now := time.Date(2025, 6, 12, 12, 0, 0, 0, time.UTC)
	quoteText := "A local quote https://t.co/quote"
	quoteStart := utf16Length("A local quote ")
	quoteEnd := utf16Length(quoteText)
	tweets := []any{
		demoEnvelope("1001", now, "A normal local post", nil),
		demoEnvelope("1002", now.Add(-time.Hour), "@friend A local reply", map[string]any{
			"in_reply_to_status_id_str": "9001", "in_reply_to_screen_name": "friend",
		}),
		demoEnvelope("1003", now.Add(-2*time.Hour), quoteText, map[string]any{
			"entities": map[string]any{"urls": []any{map[string]any{
				"url": "https://t.co/quote", "expanded_url": "https://x.com/friend/status/9002", "indices": []int{quoteStart, quoteEnd},
			}}},
		}),
		demoEnvelope("1004", now.Add(-3*time.Hour), "RT @friend: A structural repost", map[string]any{
			"entities": map[string]any{"user_mentions": []any{map[string]any{"screen_name": "friend", "indices": []int{3, 10}}}},
		}),
		demoEnvelope("1005", now.Add(-4*time.Hour), "A post with a local photo and video", map[string]any{
			"extended_entities": map[string]any{"media": []any{
				map[string]any{"id_str": "7001", "type": "photo", "media_url_https": "https://pbs.twimg.com/media/image.png", "sizes": map[string]any{"large": map[string]any{"w": 1, "h": 1}}},
				map[string]any{"id_str": "7002", "type": "video", "video_info": map[string]any{"duration_millis": 1200, "variants": []any{map[string]any{"bitrate": 256000, "content_type": "video/mp4", "url": "https://video.twimg.com/demo/clip.mp4"}}}},
			}},
		}),
		demoEnvelope("1001", now, "Duplicate record ignored", nil),
		map[string]any{"tweet": map[string]any{"id_str": "", "full_text": "Malformed synthetic record"}},
	}
	community := []any{demoEnvelope("2001", now.Add(-5*time.Hour), "A current community post", nil)}

	if err := writeDemoJS(archive, "data/account.js", "window.YTD.account.part0", account); err != nil {
		return closeWithError(err)
	}
	if err := writeDemoJS(archive, "data/tweets.js", "window.YTD.tweets.part0", tweets); err != nil {
		return closeWithError(err)
	}
	if err := writeDemoJS(archive, "data/community-tweet.js", "window.YTD.community_tweet.part0", community); err != nil {
		return closeWithError(err)
	}
	photo, err := base64.StdEncoding.DecodeString(demoPNGBase64)
	if err != nil {
		return closeWithError(err)
	}
	video, err := base64.StdEncoding.DecodeString(demoMP4Base64)
	if err != nil {
		return closeWithError(err)
	}
	if err := writeDemoEntry(archive, "data/tweets_media/1005-image.png", photo); err != nil {
		return closeWithError(err)
	}
	if err := writeDemoEntry(archive, "data/tweets_media/1005-clip.mp4", video); err != nil {
		return closeWithError(err)
	}
	return closeWithError(nil)
}

const demoPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

const demoMP4Base64 = "AAAAIGZ0eXBpc29tAAACAGlzb21pc28yYXZjMW1wNDEAAAMVbW9vdgAAAGxtdmhkAAAAAAAAAAAAAAAAAAAD6AAAACgAAQAAAQAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAgAAAj90cmFrAAAAXHRraGQAAAADAAAAAAAAAAAAAAABAAAAAAAAACgAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAABAAAAAABAAAAAQAAAAAAAkZWR0cwAAABxlbHN0AAAAAAAAAAEAAAAoAAAAAAABAAAAAAG3bWRpYQAAACBtZGhkAAAAAAAAAAAAAAAAAAAyAAAAAgBVxAAAAAAALWhkbHIAAAAAAAAAAHZpZGUAAAAAAAAAAAAAAABWaWRlb0hhbmRsZXIAAAABYm1pbmYAAAAUdm1oZAAAAAEAAAAAAAAAAAAAACRkaW5mAAAAHGRyZWYAAAAAAAAAAQAAAAx1cmwgAAAAAQAAASJzdGJsAAAAvnN0c2QAAAAAAAAAAQAAAK5hdmMxAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAABAAEABIAAAASAAAAAAAAAABFUxhdmM2Mi4yOC4xMDEgbGlieDI2NAAAAAAAAAAAAAAAGP//AAAANGF2Y0MBZAAK/+EAF2dkAAqs2V7ARAAAAwAEAAADAMg8SJZYAQAGaOvjyyLA/fj4AAAAABBwYXNwAAAAAQAAAAEAAAAUYnRydAAAAAAAAi0IAAAAAAAAABhzdHRzAAAAAAAAAAEAAAABAAACAAAAABxzdHNjAAAAAAAAAAEAAAABAAAAAQAAAAEAAAAUc3RzegAAAAAAAALJAAAAAQAAABRzdGNvAAAAAAAAAAEAAANFAAAAYnVkdGEAAABabWV0YQAAAAAAAAAhaGRscgAAAAAAAAAAbWRpcmFwcGwAAAAAAAAAAAAAAAAtaWxzdAAAACWpdG9vAAAAHWRhdGEAAAABAAAAAExhdmY2Mi4xMi4xMDEAAAAIZnJlZQAAAtFtZGF0AAACrgYF//+q3EXpvebZSLeWLNgg2SPu73gyNjQgLSBjb3JlIDE2NSByMzIyMyAwNDgwY2IwIC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyNSAtIGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTEgcmVmPTMgZGVibG9jaz0xOjA6MCBhbmFseXNlPTB4MzoweDExMyBtZT1oZXggc3VibWU9NyBwc3k9MSBwc3lfcmQ9MS4wMDowLjAwIG1peGVkX3JlZj0xIG1lX3JhbmdlPTE2IGNocm9tYV9tZT0xIHRyZWxsaXM9MSA4eDhkY3Q9MSBjcW09MCBkZWFkem9uZT0yMSwxMSBmYXN0X3Bza2lwPTEgY2hyb21hX3FwX29mZnNldD0tMiB0aHJlYWRzPTEgbG9va2FoZWFkX3RocmVhZHM9MSBzbGljZWRfdGhyZWFkcz0wIG5yPTAgZGVjaW1hdGU9MSBpbnRlcmxhY2VkPTAgYmx1cmF5X2NvbXBhdD0wIGNvbnN0cmFpbmVkX2ludHJhPTAgYmZyYW1lcz0zIGJfcHlyYW1pZD0yIGJfYWRhcHQ9MSBiX2JpYXM9MCBkaXJlY3Q9MSB3ZWlnaHRiPTEgb3Blbl9nb3A9MCB3ZWlnaHRwPTIga2V5aW50PTI1MCBrZXlpbnRfbWluPTI1IHNjZW5lY3V0PTQwIGludHJhX3JlZnJlc2g9MCByY19sb29rYWhlYWQ9NDAgcmM9Y3JmIG1idHJlZT0xIGNyZj0yMy4wIHFjb21wPTAuNjAgcXBtaW49MCBxcG1heD02OSBxcHN0ZXA9NCBpcF9yYXRpbz0xLjQwIGFxPTE6MS4wMACAAAAAE2WIhAAr//7Y5/MsiI4q33Typds="

func demoEnvelope(id string, createdAt time.Time, text string, extra map[string]any) map[string]any {
	tweet := map[string]any{
		"id_str": id, "created_at": createdAt.Format(time.RubyDate), "full_text": text,
		"entities": map[string]any{"urls": []any{}, "user_mentions": []any{}, "media": []any{}},
	}
	for key, value := range extra {
		tweet[key] = value
	}
	return map[string]any{"tweet": tweet}
}

func writeDemoJS(archive *zip.Writer, name, wrapper string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data := append([]byte(wrapper+" = "), payload...)
	data = append(data, ';', '\n')
	return writeDemoEntry(archive, name, data)
}

func writeDemoEntry(archive *zip.Writer, name string, data []byte) error {
	entry, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = entry.Write(data)
	return err
}

func utf16Length(value string) int {
	return len(utf16.Encode([]rune(value)))
}
