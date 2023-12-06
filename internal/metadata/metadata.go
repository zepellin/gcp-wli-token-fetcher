package metadata

import (
	"fmt"
	"io"
	"net/http"

	"gcp-wli-token-fetcher/internal/log"
)

func GetGSAToken(GSAName, audience, scope string) ([]byte, error) {
	log.Logger.Debug(fmt.Sprintf("Retrieving token for service account %s with audience %s and scope %s", GSAName, audience, scope))

	requestURL := fmt.Sprintf("http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/%s/identity?audience=%s&scope=%s", GSAName, audience, scope)

	client := &http.Client{}
	req, _ := http.NewRequest("GET", requestURL, nil)
	req.Header.Set("Metadata-Flavor", "Google")
	res, err := client.Do(req)
	if err != nil {
		return []byte{}, err
	}
	defer res.Body.Close()

	if b, err := io.ReadAll(res.Body); err == nil {
		return b, nil
	}
	log.Logger.Debug(fmt.Sprintf("Retrieved token for service account %s with audience %s and scope %s successfully", GSAName, audience, scope))
	return []byte{}, err
}
