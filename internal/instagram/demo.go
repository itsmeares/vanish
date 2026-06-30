package instagram

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// CreateDemoExportZIP writes a tiny fake Instagram export ZIP for local manual
// testing. The data is intentionally synthetic and non-sensitive.
func CreateDemoExportZIP(dir string) (string, error) {
	file, err := os.CreateTemp(dir, "vanish-instagram-demo-*.zip")
	if err != nil {
		return "", fmt.Errorf("create demo instagram export: %w", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	for name, body := range demoFiles() {
		entry, err := writer.Create(name)
		if err != nil {
			writer.Close()
			return "", fmt.Errorf("create demo zip entry %s: %w", name, err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			writer.Close()
			return "", fmt.Errorf("write demo zip entry %s: %w", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("finish demo instagram export: %w", err)
	}

	return file.Name(), nil
}

func demoFiles() map[string]string {
	return map[string]string{
		"your_instagram_activity/likes/liked_posts.json":        demoJSON(map[string]any{"likes_media_likes": demoLikeRecords()}),
		"your_instagram_activity/comments/post_comments_1.json": demoJSON(map[string]any{"comments_media_comments": demoCommentRecords()}),
		"followers_and_following/following.json":                demoJSON(map[string]any{"relationships_following": demoRelationshipRecords("following", 0)}),
		"followers_and_following/followers_1.json":              demoJSON(map[string]any{"relationships_followers": demoRelationshipRecords("follower", 12)}),
		"settings/unknown_shape.json":                           demoJSON(map[string]any{"unexpected": true, "note": "This sample file should be skipped with a warning."}),
		"your_instagram_activity/saved/unsupported_saved.json":  demoJSON(map[string]any{"saved_media": []any{map[string]any{"title": "saved recipe reference"}}}),
	}
}

func demoLikeRecords() []map[string]any {
	users := []string{"mira_studio", "trail_notes", "city_archive", "quiet_pages", "night_builds", "plant_shelf"}
	posts := []string{"C7mug01", "C7trail02", "C7city03", "C7book04", "C7code05", "C7plant06"}
	records := make([]map[string]any, 0, len(users))
	for i, user := range users {
		records = append(records, map[string]any{
			"title": user,
			"string_list_data": []map[string]any{
				{
					"href":      fmt.Sprintf("https://www.instagram.com/p/%s/", posts[i]),
					"value":     user,
					"timestamp": demoTimestamp(i),
				},
			},
		})
	}
	return records
}

func demoCommentRecords() []map[string]any {
	owners := []string{"rise_bakery", "paper_museum", "coastal_walks", "tiny_studio", "archive_room", "garden_notes"}
	posts := []string{"C8roll01", "C8poster02", "C8coast03", "C8sketch04", "C8photo05", "C8seed06"}
	comments := []string{
		"Loved the morning pastry.",
		"Great poster archive.",
		"Saving this beach walk.",
		"Nice linework.",
		"These photos hold up.",
		"Seedlings look strong.",
	}
	records := make([]map[string]any, 0, len(owners))
	for i, owner := range owners {
		records = append(records, map[string]any{
			"media_owner": owner,
			"string_map_data": map[string]map[string]any{
				"Comment": {
					"value":     comments[i],
					"href":      fmt.Sprintf("https://www.instagram.com/p/%s/", posts[i]),
					"timestamp": demoTimestamp(i + 6),
				},
				"Media Owner": {
					"value": owner,
				},
			},
		})
	}
	return records
}

func demoRelationshipRecords(kind string, offset int) []map[string]any {
	names := []string{"old_band", "local_gallery", "recipe_notes", "indie_dev_log", "camera_roll", "bike_maps"}
	if kind == "follower" {
		names = []string{"marina_reads", "runner_alex", "photo_walks", "neighbor_notes", "field_sketches", "market_friend"}
	}

	records := make([]map[string]any, 0, len(names))
	for i, name := range names {
		records = append(records, map[string]any{
			"title": "",
			"string_list_data": []map[string]any{
				{
					"href":      fmt.Sprintf("https://www.instagram.com/%s/", name),
					"value":     name,
					"timestamp": demoTimestamp(offset + i),
				},
			},
		})
	}
	return records
}

func demoTimestamp(offset int) int64 {
	base := time.Date(2024, 4, 8, 12, 0, 0, 0, time.UTC)
	return base.AddDate(0, 0, offset*3).Unix()
}

func demoJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("marshal demo instagram export: %v", err))
	}
	return string(data)
}
