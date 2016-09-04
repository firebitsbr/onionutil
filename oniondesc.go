// oniondesc.go - deal with onion service descriptors
//
// To the extent possible under law, Ivan Markin waived all copyright
// and related or neighboring rights to this module of onionutil, using the creative
// commons "cc0" public domain dedication. See LICENSE or
// <http://creativecommons.org/publicdomain/zero/1.0/> for full details.

package onionutil

import (
    "fmt"
    "strconv"
    "crypto/rsa"
    "crypto/sha1"
    "bytes"
    "time"
    "strings"
    "encoding/binary"
    "encoding/pem"
    "github.com/nogoegst/onionutil/torparse"
    "github.com/nogoegst/onionutil/pkcs1"
    "log"
)


type OnionDescriptor struct {
    DescId  []byte
    Version int
    PermanentKey    *rsa.PublicKey
    SecretIdPart    []byte
    PublicationTime time.Time
    ProtocolVersions    []int
    IntroductionPoints  []IntroductionPoint
    Signature   []byte
}

// Initialize defaults
func NewOnionDescriptor(perm_pk *rsa.PublicKey,
                       ips []IntroductionPoint, replica int,
                       ) (desc *OnionDescriptor) {
    desc = new(OnionDescriptor)
    /* v hardcoded values */
    desc.Version = 2
    desc.ProtocolVersions = []int{2, 3}
    /* ^ hardcoded values */
    current_time := time.Now().Unix()
    rounded_current_time := current_time-current_time%(60*60)
    desc.PublicationTime = time.Unix(rounded_current_time, 0)
    desc.PermanentKey = perm_pk
    perm_id, _ := CalcPermanentId(desc.PermanentKey)
    desc.SecretIdPart = calcSecretId(perm_id, current_time, byte(replica))
    desc.DescId = calcDescriptorId(perm_id, desc.SecretIdPart)
    desc.IntroductionPoints = ips
    return desc
}


// TODO return a pointer to descs not descs themselves?
func ParseOnionDescriptors(descs_str string) (descs []OnionDescriptor, rest string) {
    docs, _rest := torparse.ParseTorDocument([]byte(descs_str))
    for _, doc := range docs {
        var desc OnionDescriptor
        if _, ok := doc["rendezvous-service-descriptor"]; !ok {
            log.Printf("Got a document that is not an onion service")
            continue
        } else {
	    desc.DescId = doc["rendezvous-service-descriptor"].FJoined()
	}

        version, err := strconv.ParseInt(string(doc["version"].FJoined()), 10, 0)
        if err != nil {
            log.Printf("Error parsing descriptor version: %v", err)
            continue
        }
        desc.Version = int(version)

        permanent_key, _, err := pkcs1.DecodePublicKeyDER(doc["permanent-key"].FJoined())
        if err != nil {
            log.Printf("Decoding DER sequence of PulicKey has failed: %v.", err)
            continue
        }
        desc.PermanentKey = permanent_key
        //if (doc.Fields["introduction-points"]) {
            desc.IntroductionPoints, _ = ParseIntroPoints(
                                        string(doc["introduction-points"].FJoined()))
        //}
        if len(doc["signature"][0]) < 1 {
            log.Printf("Empty signature")
            continue
        }
        desc.Signature = doc["signature"].FJoined()

        // XXX: Check the signature? And strore unparsed original??

        descs = append(descs, desc)
    }

    rest = string(_rest)
    return descs, rest
}

func (desc OnionDescriptor) Body() []byte {
    w := new(bytes.Buffer)
    perm_pk_der, err := pkcs1.EncodePublicKeyDER(desc.PermanentKey)
    if err != nil {
        log.Fatalf("Cannot encode public key into DER sequence.")
    }
    fmt.Fprintf(w, "rendezvous-service-descriptor %s\n", Base32Encode(desc.DescId))
    fmt.Fprintf(w, "version %d\n", desc.Version)
    fmt.Fprintf(w, "permanent-key\n%s",
                              pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY",
                                                              Bytes: perm_pk_der}))
    fmt.Fprintf(w, "secret-id-part %s\n",
                              Base32Encode(desc.SecretIdPart))
    fmt.Fprintf(w, "publication-time %v\n",
                              desc.PublicationTime.Format("2006-01-02 15:04:05"))
    var protoversions_strs []string
    for _, v := range desc.ProtocolVersions {
        protoversions_strs = append(protoversions_strs, fmt.Sprintf("%d", v))
    }
    fmt.Fprintf(w, "protocol-versions %v\n",
                              strings.Join(protoversions_strs, ","))
    if len(desc.IntroductionPoints) != 0 {
        intro_block := MakeIntroPointsDocument(desc.IntroductionPoints)
        fmt.Fprintf(w, "introduction-points\n%s",
                                  pem.EncodeToMemory(&pem.Block{Type: "MESSAGE",
                                        Bytes: []byte(intro_block)}))
    }
    fmt.Fprintf(w, "signature\n")
    return w.Bytes()
}

func (desc *OnionDescriptor) Sign(doSign func(digest []byte) ([]byte, error)) (signedDesc []byte) {
    descBody := desc.Body()
    descDigest := CalcDescriptorBodyDigest(descBody)
    signature, err := doSign(descDigest)
    //signature, err := keycity.SignPlease(front_onion, desc_digest)
    //signature, err := signDescriptorBodyDigest(desc_digest, front_onion)
    if err != nil {
        log.Fatalf("Cannot sign: %v.", err)
    }
    signedDesc = append(descBody, encodeSignature(signature)...)
    return signedDesc
}

func encodeSignature(signature []byte) []byte {
    return pem.EncodeToMemory(&pem.Block{Type: "SIGNATURE",
                                                   Bytes: signature})
}

/* TODO: there is no `descriptor-cookie` now (because we need IP list encryption etc) */

func calcSecretId(perm_id []byte, current_time int64, replica byte) (secret_id []byte) {
    perm_id_byte := uint32(perm_id[0])

    time_period_int := (uint32(current_time) + perm_id_byte*86400/256)/86400
    var time_period = new(bytes.Buffer)
    binary.Write(time_period, binary.BigEndian, time_period_int)

    secret_id_h := sha1.New()
    secret_id_h.Write(time_period.Bytes())
    secret_id_h.Write([]byte{replica})
    secret_id = secret_id_h.Sum(nil)
    return secret_id
}

func calcDescriptorId(perm_id, secret_id []byte) (desc_id []byte){
    desc_id_h := sha1.New()
    desc_id_h.Write(perm_id)
    desc_id_h.Write(secret_id)
    desc_id_bin := desc_id_h.Sum(nil)
    return desc_id_bin
}
func CalcDescriptorBodyDigest(descBody []byte) (digest []byte) {
    return Hash(descBody)
}
