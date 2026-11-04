package executorpreflight

// The beta.6 initial update path is deliberately a strict prebuilt,
// single-service image replacement, not a general Compose orchestrator.
// ValidateNormalizedFeatureSubset operates on Docker Compose's *effective*
// JSON config, not raw YAML. It is a necessary filter, never sufficient to
// authorize Docker writes: source inclusion/implicit input discovery still
// requires independently pinned trusted input provenance.
func ValidateNormalizedFeatureSubset(config map[string]any) error {
 if len(config)==0 { return ErrNotImageOnly }
 top := map[string]bool{"name":true,"services":true,"networks":true,"volumes":true}
 for key := range config {
  if !top[key] {return ErrNotImageOnly}
 }
 services,ok:=config["services"].(map[string]any)
 if !ok||len(services)==0 {return ErrNotImageOnly}
 // Explicit positive list ensures a later Compose version cannot silently
 // enable a new container feature through normalization.
 allowed:=map[string]bool{
  "image":true, "pull_policy":true, "command":true, "entrypoint":true,
  "expose":true, "ports":true, "environment":true, "labels":true,
  "networks":true, "restart":true, "healthcheck":true,
  "working_dir":true, "user":true, "hostname":true,
  "stop_signal":true, "stop_grace_period":true, "init":true,
  "platform":true, "read_only":true, "logging":true,
 }
 for _,v:=range services {
  service,ok:=v.(map[string]any)
  if !ok||unsupportedService(service){return ErrNotImageOnly}
  for key:=range service{
   if !allowed[key] {return ErrNotImageOnly}
  }
  // First beta path intentionally excludes stateful/host bind mounts until
  // persistent data semantics are separately tested and supported.
  if _,exists:=service["volumes"];exists{return ErrNotImageOnly}
  if platform,exists:=service["platform"];exists&&platform!="linux/amd64"{
   return ErrNotImageOnly
  }
 }
 // Avoid implicit creation or drift of project networking. The initial
 // service update is scoped to pre-existing *external* networks only.
 if raw,has:=config["networks"];has{
  networks,ok:=raw.(map[string]any)
  if !ok{return ErrNotImageOnly}
  for _,v:=range networks{
   network,ok:=v.(map[string]any)
   if !ok||network["external"]!=true{return ErrNotImageOnly}
   for key:=range network{
    if key!="name"&&key!="external"&&key!="ipam"{return ErrNotImageOnly}
   }
  }
 }
 if raw,has:=config["volumes"];has{
  volumes,ok:=raw.(map[string]any)
  if !ok||len(volumes)!=0{return ErrNotImageOnly}
 }
 return nil
}
