// intropoint.go - deal with intopoints
//
// To the extent possible under law, Ivan Markin waived all copyright
// and related or neighboring rights to this module of onionutil, using the creative
// commons "cc0" public domain dedication. See LICENSE or
// <http://creativecommons.org/publicdomain/zero/1.0/> for full details.

package onionutil

import (
    "bytes"
    "log"
    "net"
    "fmt"
    "encoding/pem"
    "crypto/rsa"
    "github.com/nogoegst/onionutil/torparse"
    "github.com/nogoegst/onionutil/pkcs1"
)

type IntroductionPoint struct {
    Identity  []byte
    InternetAddress   net.IP
    OnionPort   uint16
    OnionKey    *rsa.PublicKey
    ServiceKey  *rsa.PublicKey
}


func ParseIntroPoints(ips_str string) (ips []IntroductionPoint, rest string) {
    docs, _rest := torparse.ParseTorDocument([]byte(ips_str))
    for _, doc := range docs {
        if _, ok := doc["introduction-point"]; !ok {
            log.Printf("Got a document that is not an introduction point")
            continue
        }
        var ip IntroductionPoint

        identity, err := Base32Decode(string(doc["introduction-point"].FJoined()))
        if err != nil {
            log.Printf("The IP has invalid idenity. Skipping")
            continue
        }
        ip.Identity = identity

        ip.InternetAddress = net.ParseIP(string(doc["ip-address"].FJoined()))
        if ip.InternetAddress == nil {
            log.Printf("Not a valid Internet address for an IntroPoint")
            continue
        }
        onion_port, err := InetPortFromByteString(doc["onion-port"].FJoined())
        if err != nil {
            log.Printf("Error parsing IP port: %v", err)
            continue
        }
        ip.OnionPort = onion_port
        onion_key, _, err := pkcs1.DecodePublicKeyDER(doc["onion-key"].FJoined())
        if err != nil {
            log.Printf("Decoding DER sequence of PulicKey has failed: %v.", err)
            continue
        }
        ip.OnionKey = onion_key
        service_key, _, err := pkcs1.DecodePublicKeyDER(doc["service-key"].FJoined())
        if err != nil {
            log.Printf("Decoding DER sequence of PulicKey has failed: %v.", err)
            continue
        }
        ip.ServiceKey = service_key

        ips = append(ips, ip)
    }
    rest = string(_rest)
    return ips, rest
}


// XXX: This should be gone since it just a bunch of TorDocuments
func TearApartIntroPoints(ipsEncoded []byte) (ips [][]byte) {
    title := []byte("introduction-point")
    ips = bytes.Split(ipsEncoded, title)[1:]
    for index, ip := range ips {
        ips[index] = bytes.Trim(append(title, ip...), "\n")
    }
    return ips
}

func (ip IntroductionPoint) Encode() (encodedIP []byte) {
    w := new(bytes.Buffer)
    fmt.Fprintf(w, "introduction-point %v\n", Base32Encode(ip.Identity))
    fmt.Fprintf(w, "ip-address %v\n", ip.InternetAddress)
    fmt.Fprintf(w, "onion-port %v\n", ip.OnionPort)
    onionKeyDER, err := pkcs1.EncodePublicKeyDER(ip.OnionKey)
    if err != nil {
        log.Fatalf("Cannot encode public key into DER sequence.")
    }
    onionKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY",
                                                   Bytes: onionKeyDER})
    fmt.Fprintf(w, "onion-key\n%s", onionKeyPEM)
    serviceKeyDER, err := pkcs1.EncodePublicKeyDER(ip.ServiceKey)
    if err != nil {
        log.Fatalf("Cannot encode public key into DER sequence.")
    }
    serviceKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY",
                                                   Bytes: serviceKeyDER})
    fmt.Fprintf(w, "service-key\n%s", serviceKeyPEM)

    return w.Bytes()
}

func MakeIntroPointsDocument(ips []IntroductionPoint) (ipsDoc []byte) {
    for _, ip := range ips {
        ipsDoc = append(ipsDoc, ip.Encode()...)
    }
    return ipsDoc
}
