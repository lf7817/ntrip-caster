// Package sourcetable generates the NTRIP Sourcetable response.
package sourcetable

import (
	"fmt"
	"strings"

	"ntrip-caster/internal/mountpoint"
)

// Generate builds the sourcetable body from the currently enabled mountpoints.
// The response follows the NTRIP Sourcetable format:
//
//	SOURCETABLE 200 OK\r\n
//	STR;name;...fields...\r\n
//	ENDSOURCETABLE\r\n
func Generate(mgr *mountpoint.Manager) string {
	var sb strings.Builder
	sb.WriteString("SOURCETABLE 200 OK\r\n")

	for _, mp := range mgr.List() {
		if !mp.IsEnabled() {
			continue
		}
		// STR record: STR;mountpoint;identifier;format;format-details;carrier;
		//   nav-system;network;country;lat;lon;nmea;solution;generator;compr;auth;fee;bitrate;misc
		line := fmt.Sprintf("STR;%s;%s;%s;;;;;;;;;0;;;;;",
			mp.Name, mp.Description, mp.Format)
		sb.WriteString(line)
		sb.WriteString("\r\n")
	}

	sb.WriteString("ENDSOURCETABLE\r\n")
	return sb.String()
}
