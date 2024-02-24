package main

const digits = "0123456789abcdef"

type IP []byte

func (ip IP) string4() string {
	const max = len("255.255.255.255")
	ret := make([]byte, 0, max)
	ret = ip.appendTo4(ret)
	return string(ret)
}

func (ip IP) appendTo4(ret []byte) []byte {
	ret = appendDecimal(ret, ip[0])
	ret = append(ret, '.')
	ret = appendDecimal(ret, ip[1])
	ret = append(ret, '.')
	ret = appendDecimal(ret, ip[2])
	ret = append(ret, '.')
	ret = appendDecimal(ret, ip[3])
	return ret
}

// appendDecimal appends the decimal string representation of x to b.
func appendDecimal(b []byte, x uint8) []byte {
	// Using this function rather than strconv.AppendUint makes IPv4
	// string building 2x faster.

	if x >= 100 {
		b = append(b, digits[x/100])
	}
	if x >= 10 {
		b = append(b, digits[x/10%10])
	}
	return append(b, digits[x%10])
}
