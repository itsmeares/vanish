package instagram

import (
	"archive/zip"
	"fmt"
	"os"
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
		"your_instagram_activity/likes/liked_posts.json": `{
  "likes_media_likes": [
    {
      "title": "vanish_demo_artist",
      "string_list_data": [
        {
          "href": "https://www.instagram.com/p/demo_like/",
          "value": "vanish_demo_artist",
          "timestamp": 1710000000
        }
      ]
    }
  ]
}`,
		"your_instagram_activity/comments/post_comments_1.json": `{
  "comments_media_comments": [
    {
      "media_owner": "demo_bakery",
      "string_map_data": {
        "Comment": {
          "value": "Looks amazing from fake test data!",
          "timestamp": 1710003600
        },
        "Media Owner": {
          "value": "demo_bakery"
        }
      }
    }
  ]
}`,
		"followers_and_following/following.json": `{
  "relationships_following": [
    {
      "title": "",
      "string_list_data": [
        {
          "href": "https://www.instagram.com/demo_following/",
          "value": "demo_following",
          "timestamp": 1710007200
        }
      ]
    }
  ]
}`,
		"followers_and_following/followers_1.json": `[
  {
    "title": "",
    "string_list_data": [
      {
        "href": "https://www.instagram.com/demo_follower/",
        "value": "demo_follower",
        "timestamp": 1710010800
      }
    ]
  }
]`,
		"settings/unknown_shape.json": `{
  "unexpected": true,
  "note": "This fake file should be skipped with a warning."
}`,
	}
}
