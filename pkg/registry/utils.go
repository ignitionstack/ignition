package registry

import "time"

func TruncateDigest(digest string, length int) string {
	if len(digest) <= length {
		return digest
	}
	return digest[:length]
}

func HasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func RemoveTagFromVersions(versions *[]VersionInfo, tag string) {
	for i := range *versions {
		(*versions)[i].Tags = RemoveTag((*versions)[i].Tags, tag)
	}
}

func AddTagToVersion(versions *[]VersionInfo, shortDigest, tag string) {
	for i := range *versions {
		if (*versions)[i].Hash == shortDigest {
			(*versions)[i].Tags = append((*versions)[i].Tags, tag)
			break
		}
	}
}

func RemoveTag(tags []string, tagToRemove string) []string {
	result := make([]string, 0, len(tags))
	for _, t := range tags {
		if t != tagToRemove {
			result = append(result, t)
		}
	}
	return result
}

func CreateVersionInfo(shortDigest, fullDigest string, payload []byte, tag string) VersionInfo {
	tags := []string{}
	if tag != "" {
		tags = append(tags, tag)
	}

	return VersionInfo{
		Hash:       shortDigest,
		FullDigest: fullDigest,
		CreatedAt:  time.Now(),
		Size:       int64(len(payload)),
		Tags:       tags,
	}
}
