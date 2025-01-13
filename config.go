package servotron

import (
	"encoding/json"
	"fmt"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

type Config struct {
	// file
	Debug              bool
	ListenPort         string
	ManagementPort     string
	DBConnString       string
	DBPoolSize         int
	DBQueryTimeout int
	AppUserAuth        map[string]string
	AppUserLocalParams map[string]string
	SQLRoot            string
	FileServers        map[string]string
	TemplateServers    map[string]string
	QueryStringAsJSON  bool
	// runtime
	QueryParams map[string][]string
	Routes      []Route
}

func (c *Config) String() string {
	return fmt.Sprintf("%+v", *c)
}

func (c *Config) Parse(b []byte) error {
	c.Debug = false
	c.ListenPort = "80"
	c.DBConnString = "postgresql://postgres@localhost:5432/postgres"
	c.DBPoolSize = runtime.NumCPU()
	c.DBQueryTimeout = 60
	c.AppUserAuth = make(map[string]string)
	c.AppUserAuth["Claim"] = ""
	c.AppUserAuth["Name"] = ""
	c.AppUserLocalParams = make(map[string]string)
	c.QueryStringAsJSON = true
	err := json.Unmarshal(b, &c)
	if err != nil {
		return err
	}
	who, err := user.Current()
	if err != nil {
		return err
	}
	c.SQLRoot, err = c.ResolveUserDir(who.HomeDir, c.SQLRoot)
	if err != nil {
		return err
	}
	for key, val := range c.FileServers {
		c.FileServers[key], err = c.ResolveUserDir(who.HomeDir, val)
		if err != nil {
			return err
		}
	}
	for key, val := range c.TemplateServers {
		c.TemplateServers[key], err = c.ResolveUserDir(who.HomeDir, val)
		if err != nil {
			return err
		}
	}
	for key, val := range c.AppUserLocalParams {
		c.AppUserLocalParams[key], err = c.ResolveUserDir(who.HomeDir, val)
		if err != nil {
			return err
		}
	}
	return err
}

func (c *Config) ResolveUserDir(homeDir, path string) (string, error) {
	result := path
	if strings.HasPrefix(path, "~/") {
		result = filepath.Join(homeDir, path[2:])
	}
	return filepath.Abs(result)
}
