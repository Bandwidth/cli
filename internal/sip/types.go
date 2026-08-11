package sip

import "encoding/xml"

// --- Wire types (XML) -------------------------------------------------------
// These mirror the Dashboard API exactly, including element order, which the
// generic map-based XML helper cannot express.

type realmRequest struct {
	XMLName     xml.Name `xml:"Realm"`
	Realm       string   `xml:"Realm"`
	Description string   `xml:"Description,omitempty"`
	Default     bool     `xml:"Default"` // required by the API; never omitempty
}

type realmWire struct {
	ID                 string `xml:"Id"`
	Realm              string `xml:"Realm"` // the FQDN on read responses
	Description        string `xml:"Description"`
	Default            bool   `xml:"Default"`
	SipCredentialCount int    `xml:"SipCredentialCount"`
	Status             string `xml:"Status"`
}

type realmResponse struct {
	XMLName        xml.Name        `xml:"RealmResponse"`
	Realm          *realmWire      `xml:"Realm"`
	ResponseStatus *responseStatus `xml:"ResponseStatus"`
}

type realmsResponse struct {
	XMLName        xml.Name        `xml:"RealmsResponse"`
	Realms         []realmWire     `xml:"Realms>Realm"`
	ResponseStatus *responseStatus `xml:"ResponseStatus"`
}

// credentialCreateRequest is the bulk create body. The CLI always sends one.
type credentialCreateRequest struct {
	XMLName     xml.Name              `xml:"SipCredentials"`
	Credentials []credentialCreateOne `xml:"SipCredential"`
}

type credentialCreateOne struct {
	UserName string `xml:"UserName"`
	Hash1    string `xml:"Hash1"`
	Hash1b   string `xml:"Hash1b"`
	AppID    string `xml:"HttpVoiceV2AppId,omitempty"`
}

// credentialRotateRequest deliberately has no UserName field: including the
// element fails with 23030 even when the value is unchanged.
type credentialRotateRequest struct {
	XMLName xml.Name `xml:"SipCredential"`
	RealmID string   `xml:"RealmId"` // required in the body despite being in the path
	Hash1   string   `xml:"Hash1"`
	Hash1b  string   `xml:"Hash1b"`
}

type credentialWire struct {
	ID       string `xml:"Id"`
	RealmID  string `xml:"RealmId"`
	UserName string `xml:"UserName"`
	Realm    string `xml:"Realm"` // FQDN
	AppID    string `xml:"HttpVoiceV2AppId"`
}

type credentialsResponse struct {
	XMLName        xml.Name         `xml:"SipCredentialsResponse"`
	Valid          []credentialWire `xml:"ValidSipCredentials>SipCredential"`
	Credentials    []credentialWire `xml:"SipCredentials>SipCredential"`
	Errors         []wireError      `xml:"Errors>Error"`
	ResponseStatus *responseStatus  `xml:"ResponseStatus"`
}

type credentialResponse struct {
	XMLName        xml.Name        `xml:"SipCredentialResponse"`
	Credential     *credentialWire `xml:"SipCredential"`
	ResponseStatus *responseStatus `xml:"ResponseStatus"`
}

type responseStatus struct {
	ErrorCode   string `xml:"ErrorCode"`
	Description string `xml:"Description"`
}

type wireError struct {
	ErrorCode   string `xml:"ErrorCode"`
	Description string `xml:"Description"`
}

// --- Domain types -----------------------------------------------------------
// Deliberately carry no hash fields, so digest material cannot reach output.

// Realm is a SIP authentication realm.
type Realm struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Hostname        string `json:"hostname"`
	Description     string `json:"description"`
	Default         bool   `json:"default"`
	Status          string `json:"status"`
	CredentialCount int    `json:"credentialCount"`
}

// Credential is a SIP digest credential belonging to a realm.
type Credential struct {
	ID       string `json:"id"`
	RealmID  string `json:"realmId"`
	Username string `json:"username"`
	Hostname string `json:"hostname"`
	AppID    string `json:"appId"`
}
