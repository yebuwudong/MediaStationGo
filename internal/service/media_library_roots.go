package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type LibraryRootInput struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Path      string `json:"path"`
	Enabled   *bool  `json:"enabled,omitempty"`
	SortOrder *int   `json:"sort_order,omitempty"`
}

// CreateLibrary persists a library after validating that its path exists.
func (s *MediaService) CreateLibrary(ctx context.Context, name, path, kind string) (*model.Library, error) {
	return s.CreateLibraryWithRoots(ctx, name, kind, []LibraryRootInput{{Path: path}})
}

func (s *MediaService) CreateLibraryWithRoots(ctx context.Context, name, kind string, inputs []LibraryRootInput) (*model.Library, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("name required")
	}
	roots, err := normalizeLibraryRootInputs(inputs, true)
	if err != nil {
		return nil, err
	}
	kind = inferLibraryKind(name, roots[0].Path, kind)
	lib := &model.Library{Name: strings.TrimSpace(name), Path: roots[0].Path, Type: kind, Enabled: true}
	if err := s.repo.Library.CreateWithRoots(ctx, lib, roots); err != nil {
		return nil, err
	}
	s.invalidateMediaCache(ctx)
	return lib, nil
}

func normalizeLibraryRootInputs(inputs []LibraryRootInput, requirePath bool) ([]model.LibraryRoot, error) {
	roots := make([]model.LibraryRoot, 0, len(inputs))
	seen := map[string]struct{}{}
	for i, input := range inputs {
		rawPath := strings.TrimSpace(input.Path)
		if rawPath == "" {
			if requirePath {
				return nil, errors.New("at least one path required")
			}
			continue
		}
		abs, err := resolveAccessibleLibraryPath(rawPath)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(filepath.Clean(abs))
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate library path: %s", abs)
		}
		seen[key] = struct{}{}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		roots = append(roots, model.LibraryRoot{
			Name:    strings.TrimSpace(input.Name),
			Path:    abs,
			Enabled: enabled,
		})
		if input.SortOrder != nil {
			roots[len(roots)-1].SortOrder = *input.SortOrder
		} else {
			roots[len(roots)-1].SortOrder = i
		}
	}
	if len(roots) == 0 && requirePath {
		return nil, errors.New("at least one path required")
	}
	return roots, nil
}

func (s *MediaService) ListLibraryRoots(ctx context.Context, libraryID string) ([]model.LibraryRoot, error) {
	if err := s.ensureLibraryRoots(ctx, libraryID); err != nil {
		return nil, err
	}
	return s.repo.Library.ListRoots(ctx, libraryID)
}

func (s *MediaService) AddLibraryRoot(ctx context.Context, libraryID string, input LibraryRootInput) (*model.LibraryRoot, error) {
	lib, err := s.repo.Library.FindByID(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		return nil, errors.New("library not found")
	}
	roots, err := normalizeLibraryRootInputs([]LibraryRootInput{input}, true)
	if err != nil {
		return nil, err
	}
	root := roots[0]
	root.LibraryID = libraryID
	if err := s.ensureLibraryRootPathUnique(ctx, libraryID, "", root.Path); err != nil {
		return nil, err
	}
	if root.SortOrder == 0 {
		existing, _ := s.repo.Library.ListRoots(ctx, libraryID)
		root.SortOrder = len(existing)
	}
	if err := s.repo.Library.CreateRoot(ctx, &root); err != nil {
		return nil, err
	}
	if strings.TrimSpace(lib.Path) == "" {
		_ = s.repo.DB.WithContext(ctx).Model(&model.Library{}).Where("id = ?", libraryID).Update("path", root.Path).Error
	}
	return &root, nil
}

func (s *MediaService) UpdateLibraryRoot(ctx context.Context, libraryID, rootID string, input LibraryRootInput) (*model.LibraryRoot, error) {
	root, err := s.repo.Library.FindRootByID(ctx, libraryID, rootID)
	if err != nil || root == nil {
		return root, err
	}
	updates := map[string]any{}
	if input.Name != "" {
		updates["name"] = strings.TrimSpace(input.Name)
	}
	if strings.TrimSpace(input.Path) != "" {
		roots, err := normalizeLibraryRootInputs([]LibraryRootInput{input}, true)
		if err != nil {
			return nil, err
		}
		if err := s.ensureLibraryRootPathUnique(ctx, libraryID, rootID, roots[0].Path); err != nil {
			return nil, err
		}
		updates["path"] = roots[0].Path
		root.Path = roots[0].Path
	}
	if input.Enabled != nil {
		updates["enabled"] = *input.Enabled
		root.Enabled = *input.Enabled
	}
	if input.SortOrder != nil {
		updates["sort_order"] = *input.SortOrder
		root.SortOrder = *input.SortOrder
	}
	if err := s.repo.Library.UpdateRoot(ctx, root, updates); err != nil {
		return nil, err
	}
	if err := s.syncLibraryPrimaryRoot(ctx, libraryID); err != nil {
		return nil, err
	}
	return s.repo.Library.FindRootByID(ctx, libraryID, rootID)
}

func (s *MediaService) DeleteLibraryRoot(ctx context.Context, libraryID, rootID string) error {
	root, err := s.repo.Library.FindRootByID(ctx, libraryID, rootID)
	if err != nil {
		return err
	}
	if root == nil {
		return errors.New("library root not found")
	}
	roots, err := s.repo.Library.ListRoots(ctx, libraryID)
	if err != nil {
		return err
	}
	if len(roots) <= 1 {
		return errors.New("library must keep at least one path")
	}
	if err := s.repo.Media.DeleteByLibraryRoot(ctx, libraryID, rootID); err != nil {
		return err
	}
	if err := s.repo.Library.DeleteRoot(ctx, libraryID, rootID); err != nil {
		return err
	}
	return s.syncLibraryPrimaryRoot(ctx, libraryID)
}

func (s *MediaService) ensureLibraryRoots(ctx context.Context, libraryID string) error {
	lib, err := s.repo.Library.FindByID(ctx, libraryID)
	if err != nil || lib == nil || len(lib.Roots) > 0 || strings.TrimSpace(lib.Path) == "" {
		return err
	}
	root := &model.LibraryRoot{
		LibraryID: libraryID,
		Name:      filepath.Base(filepath.Clean(lib.Path)),
		Path:      lib.Path,
		Enabled:   lib.Enabled,
		SortOrder: 0,
	}
	return s.repo.Library.CreateRoot(ctx, root)
}

func (s *MediaService) syncLibraryPrimaryRoot(ctx context.Context, libraryID string) error {
	roots, err := s.repo.Library.ListRoots(ctx, libraryID)
	if err != nil || len(roots) == 0 {
		return err
	}
	return s.repo.DB.WithContext(ctx).Model(&model.Library{}).Where("id = ?", libraryID).Update("path", roots[0].Path).Error
}

func (s *MediaService) ensureLibraryRootPathUnique(ctx context.Context, libraryID, exceptRootID, pathValue string) error {
	roots, err := s.repo.Library.ListRoots(ctx, libraryID)
	if err != nil {
		return err
	}
	key := strings.ToLower(filepath.Clean(strings.TrimSpace(pathValue)))
	for _, existing := range roots {
		if existing.ID == exceptRootID {
			continue
		}
		if strings.ToLower(filepath.Clean(strings.TrimSpace(existing.Path))) == key {
			return fmt.Errorf("duplicate library path: %s", pathValue)
		}
	}
	return nil
}
