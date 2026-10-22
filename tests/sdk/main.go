// Independent Compose SDK evaluation. Read-only by design; no Up or Pull.
package main

import (
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "os"

 "github.com/docker/cli/cli/command"
 "github.com/docker/cli/cli/flags"
 "github.com/docker/compose/v5/pkg/api"
 "github.com/docker/compose/v5/pkg/compose"
)

type snapshot struct {
 Name string `json:"name"`
 Services map[string]serviceView `json:"services"`
}
type serviceView struct {
 Image string `json:"image"`
 Labels map[string]string `json:"labels"`
 Environment map[string]*string `json:"environment"`
 Volumes []string `json:"volumes"`
 Networks []string `json:"networks"`
}

func run() error {
 projectDir := flag.String("project-dir", "", "absolute project directory")
 name := flag.String("project", "", "Compose project name")
 service := flag.String("service", "", "optional scoped service (load-only)")
 flag.Parse()
 if *projectDir == "" || *name == "" || len(flag.Args()) == 0 {
  return fmt.Errorf("usage: sdk-eval -project-dir DIR -project NAME [-service SERVICE] compose.yaml [override.yaml...]")
 }
 cli, err := command.NewDockerCli()
 if err != nil { return fmt.Errorf("new docker cli: %w", err) }
 if err = cli.Initialize(&flags.ClientOptions{}); err != nil { return fmt.Errorf("initialize: %w", err) }
 backend, err := compose.NewComposeService(cli)
 if err != nil { return fmt.Errorf("compose service: %w", err) }
 opts := api.ProjectLoadOptions{
  ConfigPaths: flag.Args(),
  ProjectName: *name,
  WorkingDir: *projectDir,
 }
 if *service != "" { opts.Services = []string{*service} }
 project, err := backend.LoadProject(context.Background(), opts)
 if err != nil { return fmt.Errorf("load Compose project: %w", err) }
 result := snapshot{Name: project.Name, Services: make(map[string]serviceView)}
 for key, svc := range project.Services {
  v := serviceView{
   Image: svc.Image,
   Labels: map[string]string(svc.Labels),
   Environment: map[string]*string(svc.Environment),
  }
  for _, mount := range svc.Volumes {
   v.Volumes = append(v.Volumes, mount.Source+":"+mount.Target)
  }
  for name := range svc.Networks { v.Networks = append(v.Networks, name) }
  result.Services[key] = v
 }
 return json.NewEncoder(os.Stdout).Encode(result)
}
func main() { if err := run(); err != nil { fmt.Fprintln(os.Stderr,err);os.Exit(1) } }
