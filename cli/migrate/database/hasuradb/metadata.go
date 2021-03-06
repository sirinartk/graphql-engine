package hasuradb

import (
	"encoding/json"
	"fmt"
	"net/http"

	gyaml "github.com/ghodss/yaml"
	"github.com/hasura/graphql-engine/cli/metadata/types"
	"github.com/hasura/graphql-engine/cli/migrate/database"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	"github.com/oliveagle/jsonpath"
)

func (h *HasuraDB) SetMetadataPlugins(plugins types.MetadataPlugins) {
	h.config.Plugins = plugins
}

func (h *HasuraDB) ExportMetadata() (map[string][]byte, error) {
	query := HasuraQuery{
		Type: "export_metadata",
		Args: HasuraArgs{},
	}

	resp, body, err := h.sendv1Query(query)
	if err != nil {
		h.logger.Debug(err)
		return nil, err
	}
	h.logger.Debug("response: ", string(body))

	var horror HasuraError
	if resp.StatusCode != http.StatusOK {
		err = json.Unmarshal(body, &horror)
		if err != nil {
			h.logger.Debug(err)
			return nil, fmt.Errorf("failed parsing json: %v; response from API: %s", err, string(body))
		}
		return nil, horror.Error(h.config.isCMD)
	}

	var c yaml.MapSlice
	err = yaml.Unmarshal(body, &c)
	if err != nil {
		h.logger.Debug(err)
		return nil, err
	}

	metadataFiles := make(map[string][]byte)
	for _, plg := range h.config.Plugins {
		files, err := plg.Export(c)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("cannot export %s from metadata", plg.Name()))
		}
		for fileName, content := range files {
			metadataFiles[fileName] = content
		}
	}
	return metadataFiles, nil
}

func (h *HasuraDB) ResetMetadata() error {
	query := HasuraInterfaceQuery{
		Type: "clear_metadata",
		Args: HasuraArgs{},
	}

	resp, body, err := h.sendv1Query(query)
	if err != nil {
		h.logger.Debug(err)
		return err
	}
	h.logger.Debug("response: ", string(body))

	var horror HasuraError
	if resp.StatusCode != http.StatusOK {
		err = json.Unmarshal(body, &horror)
		if err != nil {
			h.logger.Debug(err)
			return fmt.Errorf("failed parsing json: %v; response from API: %s", err, string(body))
		}
		return horror.Error(h.config.isCMD)
	}
	return nil
}

// ReloadMetadata - Reload Hasura GraphQL Engine metadata on the database
func (h *HasuraDB) ReloadMetadata() error {
	query := HasuraInterfaceQuery{
		Type: "reload_metadata",
		Args: HasuraArgs{},
	}

	resp, body, err := h.sendv1Query(query)
	if err != nil {
		h.logger.Debug(err)
		return err
	}
	h.logger.Debug("response: ", string(body))

	var horror HasuraError
	if resp.StatusCode != http.StatusOK {
		err = json.Unmarshal(body, &horror)
		if err != nil {
			h.logger.Debug(err)
			return fmt.Errorf("failed parsing json: %v; response from API: %s", err, string(body))
		}
		return horror.Error(h.config.isCMD)
	}
	return nil
}

func (h *HasuraDB) GetInconsistentMetadata() (bool, []database.InconsistentMetadataInterface, error) {
	query := HasuraInterfaceQuery{
		Type: "get_inconsistent_metadata",
		Args: HasuraArgs{},
	}

	resp, body, err := h.sendv1Query(query)
	if err != nil {
		h.logger.Debug(err)
		return false, nil, err
	}
	h.logger.Debug("response: ", string(body))

	var horror HasuraError
	if resp.StatusCode != http.StatusOK {
		err = json.Unmarshal(body, &horror)
		if err != nil {
			h.logger.Debug(err)
			return false, nil, err
		}
		return false, nil, horror.Error(h.config.isCMD)
	}

	var inMet InconsistentMetadata
	err = json.Unmarshal(body, &inMet)
	if err != nil {
		return false, nil, err
	}
	inMetInterface := make([]database.InconsistentMetadataInterface, 0)
	for _, obj := range inMet.InConsistentObjects {
		inMetInterface = append(inMetInterface, database.InconsistentMetadataInterface(obj))
	}
	return inMet.IsConsistent, inMetInterface, nil
}

func (h *HasuraDB) DropInconsistentMetadata() error {
	query := HasuraInterfaceQuery{
		Type: "drop_inconsistent_metadata",
		Args: HasuraArgs{},
	}

	resp, body, err := h.sendv1Query(query)
	if err != nil {
		h.logger.Debug(err)
		return err
	}
	h.logger.Debug("response: ", string(body))

	var horror HasuraError
	if resp.StatusCode != http.StatusOK {
		err = json.Unmarshal(body, &horror)
		if err != nil {
			h.logger.Debug(err)
			return err
		}
		return horror.Error(h.config.isCMD)
	}
	return nil
}

func (h *HasuraDB) BuildMetadata() (yaml.MapSlice, error) {
	var tmpMeta yaml.MapSlice
	for _, plg := range h.config.Plugins {
		err := plg.Build(&tmpMeta)
		if err != nil {
			return tmpMeta, errors.Wrap(err, fmt.Sprintf("cannot build %s from metadata", plg.Name()))
		}
	}
	return tmpMeta, nil
}

func (h *HasuraDB) ApplyMetadata() error {
	tmpMeta, err := h.BuildMetadata()
	if err != nil {
		return err
	}
	yByt, err := yaml.Marshal(tmpMeta)
	if err != nil {
		return err
	}
	jbyt, err := gyaml.YAMLToJSON(yByt)
	if err != nil {
		return err
	}
	var obj interface{}
	err = json.Unmarshal(jbyt, &obj)
	if err != nil {
		return err
	}
	query := HasuraInterfaceBulk{
		Type: "bulk",
		Args: []interface{}{
			HasuraInterfaceQuery{
				Type: "clear_metadata",
				Args: HasuraArgs{},
			},
			HasuraInterfaceQuery{
				Type: "replace_metadata",
				Args: obj,
			},
		},
	}
	resp, body, err := h.sendv1Query(query)
	if err != nil {
		h.logger.Debug(err)
		return err
	}
	h.logger.Debug("response: ", string(body))

	var horror HasuraError
	if resp.StatusCode != http.StatusOK {
		err = json.Unmarshal(body, &horror)
		if err != nil {
			h.logger.Debug(err)
			return fmt.Errorf("failed parsing json: %v; response from API: %s", err, string(body))
		}

		if horror.Path != "" {
			jsonData, err := json.Marshal(query)
			if err != nil {
				return err
			}
			var metadataQuery interface{}
			err = json.Unmarshal(jsonData, &metadataQuery)
			if err != nil {
				return err
			}
			lookup, err := jsonpath.JsonPathLookup(metadataQuery, horror.Path)
			if err == nil {
				queryData, err := json.MarshalIndent(lookup, "", "  ")
				if err != nil {
					return err
				}
				horror.migrationQuery = "offending object: \n\r\n\r" + string(queryData)
			}
		}
		return horror.Error(h.config.isCMD)
	}
	return nil
}

func (h *HasuraDB) Query(data interface{}) error {
	query := HasuraInterfaceQuery{
		Type: "bulk",
		Args: data,
	}

	resp, body, err := h.sendv1Query(query)
	if err != nil {
		h.logger.Debug(err)
		return err
	}
	h.logger.Debug("response: ", string(body))

	var horror HasuraError
	if resp.StatusCode != http.StatusOK {
		err = json.Unmarshal(body, &horror)
		if err != nil {
			h.logger.Debug(err)
			return fmt.Errorf("failed parsing json: %v; response from API: %s", err, string(body))
		}
		return horror.Error(h.config.isCMD)
	}
	return nil
}
