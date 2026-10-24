// Command qualify-smb-output is a disposable CI fixture, never a production
// App entrypoint or a TrueNAS mutation tool.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	frExport "github.com/SemperSupra/folio-relay/internal/export"
)

func main() {
	canon:=flag.String("canonical","","isolated test canonical directory")
	out:=flag.String("export","","isolated test managed output directory")
	flag.Parse()
	if *canon=="" || *out=="" { panic("two separate fixture directories required") }
	body:=[]byte("%PDF-1.7\nFolioRelay synthetic SMB fixture\n")
	sum:=sha256.Sum256(body)
	digest:=hex.EncodeToString(sum[:])
	location:=filepath.Join(*canon,"sha256",digest[:2],digest)
	if err:=os.MkdirAll(filepath.Dir(location),0700);err!=nil {panic(err)}
	if err:=os.WriteFile(location,body,0600);err!=nil {panic(err)}
	worker,err:=frExport.NewWorkerWithMode(*canon,*out,0660)
	if err!=nil {panic(err)}
	defer worker.Close()
	p,err:=frExport.MakePlan(frExport.Defaults{
		Folder:"Prints/2026",
		FilenameTemplate:"smb-smoke-{job_id}",
	},frExport.Overrides{},frExport.Job{
		JobID:"job-"+strings.Repeat("a",64),
		SourceTitle:"synthetic",
		AcceptedAt:time.Date(2026,10,24,0,0,0,0,time.UTC),
		MediaType:"application/pdf",
	})
	if err!=nil {panic(err)}
	r,err:=worker.Publish(p,frExport.Artifact{SHA256:digest,Bytes:int64(len(body))})
	if err!=nil {panic(err)}
	fmt.Println(r.RelativePath)
}
