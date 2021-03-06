/**
 * Copyright (C) 2015 Deepin Technology Co., Ltd.
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 3 of the License, or
 * (at your option) any later version.
 **/

package main

// #cgo pkg-config: glib-2.0
// #include <glib.h>
import "C"
import (
	"errors"
	"gir/glib-2.0"
	"net/url"
	"path/filepath"
	"pkg.deepin.io/lib/dbus"
	d "pkg.deepin.io/service/file-manager-backend/delegator"
	. "pkg.deepin.io/service/file-manager-backend/log"
	"pkg.deepin.io/service/file-manager-backend/operations"
	"strings"
	"sync"
)

var getConn = (func() func() *dbus.Conn {
	var conn *dbus.Conn
	var once sync.Once
	return func() *dbus.Conn {
		once.Do(func() {
			var err error
			conn, err = dbus.SessionBus()
			if err != nil {
				Log.Fatal("Connect to Session Bus failed:", err)
			}
		})
		return conn
	}
}())

// OperationBackend is the backend to create dbus operations for front end.
type OperationBackend struct {
}

// NewOperationBackend creates a new backend for operations.
func NewOperationBackend() *OperationBackend {
	op := &OperationBackend{}
	return op
}

func (*OperationBackend) newUIDelegate(dest string, objPath string, iface string) d.IUIDelegate {
	uiDelegate, err := d.NewUIDelegate(getConn(), dest, objPath, iface)
	if err != nil {
		Log.Warningf("Create UI Delegate from (%s, %s, %s) failed: %v", dest, objPath, iface, err)
	}
	return uiDelegate
}

// GetDBusInfo returns the dbus info which is needed to export dbus.
func (*OperationBackend) GetDBusInfo() dbus.DBusInfo {
	return dbus.DBusInfo{
		Dest:       d.JobDestination,
		ObjectPath: d.JobObjectPath,
		Interface:  d.JobDestination,
	}
}

// Because empty object path is invalid, so JobObjectPath is used as default value.
// So empty interface means install failed.
func installJob(job dbus.DBusObject) (string, dbus.ObjectPath, string, error) {
	dest := ""
	objPath := dbus.ObjectPath(d.JobObjectPath)
	iface := ""
	if job == nil {
		Log.Warning("cannot install a nil object on dbus.")
		return dest, objPath, iface, errors.New("cannot install a nil object on dbus.")
	}

	err := dbus.InstallOnSession(job)
	if err != nil {
		Log.Warning("install dbus on session bus failed:", err)
		return dest, objPath, iface, err
	}

	dbusInfo := job.GetDBusInfo()
	return dbusInfo.Dest, dbus.ObjectPath(dbusInfo.ObjectPath), dbusInfo.Interface, nil
}

// pathToURL transforms a absolute path to URL.
func pathToURL(path string) (*url.URL, error) {
	srcURL, err := url.Parse(path)
	if err != nil {
		return srcURL, err
	}

	if !filepath.IsAbs(srcURL.Path) {
		return srcURL, errors.New("a absolute path is requested")
	}

	if srcURL.Scheme == "" {
		srcURL.Scheme = "file"
	}

	return srcURL, nil
}

// create a new operation from fn and install it to session bus.
// NB: using closure and anonymous function is ok,
// but variable-length argument list is much more safer and much more portable.
func newOperationJob(paths []string, fn func([]string, ...interface{}) dbus.DBusObject, args ...interface{}) (string, dbus.ObjectPath, string, error) {
	objPath := dbus.ObjectPath(d.JobObjectPath)
	iface := ""

	srcURLs := make([]string, len(paths))
	for i, path := range paths {
		srcURL, err := pathToURL(path)
		if err != nil {
			Log.Error("convert path to URL failed:", err)
			return "", objPath, iface, err // maybe continue is a better choice.
		}
		srcURLs[i] = srcURL.String()
	}

	return installJob(fn(srcURLs, args...))
}

// NewListJob creates a new list job for front end.
func (*OperationBackend) NewListJob(path string, flags int32) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewListJob")
	return newOperationJob([]string{path}, func(uri []string, args ...interface{}) dbus.DBusObject {
		return d.NewListJob(uri[0], operations.ListJobFlag(flags))
	})
}

// NewStatJob creates a new stat job for front end.
func (*OperationBackend) NewStatJob(path string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewStatJob")
	return newOperationJob([]string{path}, func(uri []string, args ...interface{}) dbus.DBusObject {
		return d.NewStatJob(uri[0])
	})
}

// NewDeleteJob creates a new delete job for front end.
func (backend *OperationBackend) NewDeleteJob(path []string, shouldConfirm bool, dest string, objPath string, iface string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewDeleteJob")
	return newOperationJob(path, func(uri []string, args ...interface{}) dbus.DBusObject {
		uiDelegate := backend.newUIDelegate(dest, objPath, iface)
		return d.NewDeleteJob(uri, shouldConfirm, uiDelegate)
	})
}

// NewTrashJob creates a new TrashJob for front end.
func (backend *OperationBackend) NewTrashJob(path []string, shouldConfirm bool, dest string, objPath string, iface string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewTrashJob")
	return newOperationJob(path, func(uri []string, args ...interface{}) dbus.DBusObject {
		uiDelegate := backend.newUIDelegate(dest, objPath, iface)
		return d.NewTrashJob(uri, shouldConfirm, uiDelegate)
	})
}

// NewEmptyTrashJob creates a new EmptyTrashJob for front end.
func (backend *OperationBackend) NewEmptyTrashJob(shouldConfirm bool, dest string, objPath string, iface string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewEmptyTrashJob")
	return newOperationJob([]string{}, func(uris []string, args ...interface{}) dbus.DBusObject {
		uiDelegate := backend.newUIDelegate(dest, objPath, iface)
		return d.NewEmptyTrashJob(shouldConfirm, uiDelegate)
	})
}

// NewChmodJob creates a new change mode job for dbus.
func (*OperationBackend) NewChmodJob(path string, permission uint32) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewChmodJob")
	return newOperationJob([]string{path}, func(uris []string, args ...interface{}) dbus.DBusObject {
		return d.NewChmodJob(uris[0], permission)
	})
}

// NewChownJob creates a new change owner job for dbus.
func (*OperationBackend) NewChownJob(path string, newOwner string, newGroup string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewChownJob")
	return newOperationJob([]string{path}, func(uris []string, args ...interface{}) dbus.DBusObject {
		return d.NewChownJob(uris[0], newOwner, newGroup)
	})
}

// NewCreateFileJob creates a new create file job for dbus.
func (backend *OperationBackend) NewCreateFileJob(destDir string, filename string, initContent string, dest string, objPath string, iface string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewCreateFileJob")
	return newOperationJob([]string{destDir}, func(uris []string, args ...interface{}) dbus.DBusObject {
		uiDelegate := backend.newUIDelegate(dest, objPath, iface)
		return d.NewCreateFileJob(uris[0], filename, []byte(initContent), uiDelegate)
	})
}

// NewCreateDirectoryJob creates a new create directory job for dbus.
func (backend *OperationBackend) NewCreateDirectoryJob(destDir string, dirname string, dest string, objPath string, iface string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewCreateDirectoryJob")
	return newOperationJob([]string{destDir}, func(uris []string, args ...interface{}) dbus.DBusObject {
		uiDelegate := backend.newUIDelegate(dest, objPath, iface)
		return d.NewCreateDirectoryJob(uris[0], dirname, uiDelegate)
	})
}

// NewCreateFileFromTemplateJob creates a new create file job fro dbus.
func (backend *OperationBackend) NewCreateFileFromTemplateJob(destDir string, templatePath string, dest string, objPath string, iface string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewCreateFileFromTemplateJob")
	return newOperationJob([]string{destDir}, func(uris []string, args ...interface{}) dbus.DBusObject {
		templateURL, err := pathToURL(templatePath)
		if err != nil {
			Log.Error("convert path to URL failed:", err)
			return nil
		}

		uiDelegate := backend.newUIDelegate(dest, objPath, iface)
		return d.NewCreateFileFromTemplateJob(uris[0], templateURL.String(), uiDelegate)
	})
}

// NewLinkJob creates a new link job for dbus.
func (backend *OperationBackend) NewLinkJob(src string, destDir string, dest string, objPath string, iface string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewLinkJob")
	return newOperationJob([]string{src}, func(uris []string, args ...interface{}) dbus.DBusObject {
		destDirURL, err := pathToURL(destDir)
		if err != nil {
			Log.Error("convert path to URL failed:", err)
			return nil
		}

		uiDelegate := backend.newUIDelegate(dest, objPath, iface)
		return d.NewLinkJob(uris[0], destDirURL.String(), uiDelegate)
	})
}

// NewGetDefaultLaunchAppJob creates a new get launch app job for dbus.
func (*OperationBackend) NewGetDefaultLaunchAppJob(path string, mustSupportURI bool) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewGetDefaultLaunchAppJob")
	return newOperationJob([]string{path}, func(uris []string, args ...interface{}) dbus.DBusObject {
		return d.NewGetDefaultLaunchAppJob(uris[0], mustSupportURI)
	})
}

func (*OperationBackend) NewGetRecommendedLaunchAppsJob(uri string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewGetRecommendedLaunchAppsJob")
	return newOperationJob([]string{uri}, func(uris []string, args ...interface{}) dbus.DBusObject {
		return d.NewGetRecommendedLaunchAppsJob(uris[0])
	})
}

func (*OperationBackend) NewGetAllLaunchAppsJob() (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewGetAllLaunchAppsJob")
	return newOperationJob([]string{}, func(uris []string, args ...interface{}) dbus.DBusObject {
		return d.NewGetAllLaunchAppsJob()
	})
}

// NewSetDefaultLaunchAppJob creates a new set default launch app job for dbus.
func (*OperationBackend) NewSetDefaultLaunchAppJob(id string, mimeType string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewSetDefaultLaunchAppJob")
	return newOperationJob([]string{}, func(uris []string, args ...interface{}) dbus.DBusObject {
		if !strings.HasSuffix(id, ".desktop") {
			Log.Errorf("wrong desktop id: %q", id)
			return nil
		}
		return d.NewSetDefaultLaunchAppJob(id, mimeType)
	})
}

// NewCopyJob creates a new copy job for dbus.
func (backend *OperationBackend) NewCopyJob(srcs []string, destDir string, targetName string, flags uint32, dest string, objPath string, iface string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewCopyJob")
	return newOperationJob(srcs, func(uris []string, args ...interface{}) dbus.DBusObject {
		destDirURL, err := pathToURL(destDir)
		if destDir != "" && err != nil {
			Log.Error("convert path to URL failed:", err)
			return nil
		}

		uiDelegate := backend.newUIDelegate(dest, objPath, iface)
		return d.NewCopyJob(uris, destDirURL.String(), targetName, flags, uiDelegate)
	})
}

// NewMoveJob creates a new move job for dbus.
func (backend *OperationBackend) NewMoveJob(paths []string, destDir string, targetName string, flags uint32, dest string, objPath string, iface string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewMoveJob")
	return newOperationJob(paths, func(uris []string, args ...interface{}) dbus.DBusObject {
		destDirURL, err := pathToURL(destDir)
		if destDir != "" && err != nil {
			Log.Error("convert path to URL failed:", err)
			return nil
		}

		uiDelegate := backend.newUIDelegate(dest, objPath, iface)
		return d.NewMoveJob(uris, destDirURL.String(), targetName, flags, uiDelegate)
	})
}

// NewRenameJob creates a new rename job for dbus.
func (backend *OperationBackend) NewRenameJob(fileURL string, newName string) (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewRenameJob")
	return newOperationJob([]string{fileURL}, func(uris []string, args ...interface{}) dbus.DBusObject {
		fileURL := uris[0]
		return d.NewRenameJob(fileURL, newName)
	})
}

// NewGetTemplateJob creates a new get template job for dbus.
func (backend *OperationBackend) NewGetTemplateJob() (string, dbus.ObjectPath, string, error) {
	Log.Debug("NewGetTemplateJob")
	C.g_reload_user_special_dirs_cache()
	templateDirPath := glib.GetUserSpecialDir(glib.UserDirectoryDirectoryTemplates)

	return newOperationJob([]string{templateDirPath}, func(uris []string, args ...interface{}) dbus.DBusObject {
		templateDirURI := uris[0]
		return d.NewGetTemplateJob(templateDirURI)
	})
}
