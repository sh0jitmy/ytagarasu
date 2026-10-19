// Copyright 2026 [Copyright Holder]
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: [YOUR_NAME]

package supervisor

import (
	"errors"
	"fmt"
	"net"
)

// ErrPortConflict indicates that a requested port is already in use by another process.
var ErrPortConflict = errors.New("port conflict detected")

// CheckPortAvailable verifies whether a given TCP port is available on localhost.
func CheckPortAvailable(port int) error {
	if port <= 0 || port > 65535 {
		return nil
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("%w: port %d is already in use by another process on host (%v)", ErrPortConflict, port, err)
	}
	_ = l.Close()
	return nil
}
