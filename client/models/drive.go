// Code generated by go-swagger; DO NOT EDIT.

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
// 	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	strfmt "github.com/go-openapi/strfmt"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/swag"
	"github.com/go-openapi/validate"
)

// Drive drive
// swagger:model Drive
type Drive struct {

	// Represents the caching strategy for the block device.
	CacheType *string `json:"cache_type,omitempty"`

	// drive id
	// Required: true
	DriveID *string `json:"drive_id"`

	// is read only
	// Required: true
	IsReadOnly *bool `json:"is_read_only"`

	// is root device
	// Required: true
	IsRootDevice *bool `json:"is_root_device"`

	// Represents the unique id of the boot partition of this device. It is optional and it will be taken into account only if the is_root_device field is true.
	Partuuid string `json:"partuuid,omitempty"`

	// Host level path for the guest drive
	// Required: true
	PathOnHost *string `json:"path_on_host"`

	// rate limiter
	RateLimiter *RateLimiter `json:"rate_limiter,omitempty"`
}

// Validate validates this drive
func (m *Drive) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateDriveID(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateIsReadOnly(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateIsRootDevice(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validatePathOnHost(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateRateLimiter(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *Drive) validateDriveID(formats strfmt.Registry) error {

	if err := validate.Required("drive_id", "body", m.DriveID); err != nil {
		return err
	}

	return nil
}

func (m *Drive) validateIsReadOnly(formats strfmt.Registry) error {

	if err := validate.Required("is_read_only", "body", m.IsReadOnly); err != nil {
		return err
	}

	return nil
}

func (m *Drive) validateIsRootDevice(formats strfmt.Registry) error {

	if err := validate.Required("is_root_device", "body", m.IsRootDevice); err != nil {
		return err
	}

	return nil
}

func (m *Drive) validatePathOnHost(formats strfmt.Registry) error {

	if err := validate.Required("path_on_host", "body", m.PathOnHost); err != nil {
		return err
	}

	return nil
}

func (m *Drive) validateRateLimiter(formats strfmt.Registry) error {

	if swag.IsZero(m.RateLimiter) { // not required
		return nil
	}

	if m.RateLimiter != nil {
		if err := m.RateLimiter.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("rate_limiter")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *Drive) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *Drive) UnmarshalBinary(b []byte) error {
	var res Drive
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
