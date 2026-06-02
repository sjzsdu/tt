import { Button } from 'antd';
import { DeleteOutlined, EditOutlined, SaveOutlined } from '@ant-design/icons';
import type { DocumentResponse, Route } from '../types';

interface DocumentToolbarProps {
  doc: DocumentResponse;
  route: Route;
  contentMode?: boolean;
  saving: boolean;
  navigate: (href: string) => void;
  onSave: () => void;
  onDelete: () => void;
}

export function DocumentToolbar({ doc, route, contentMode, saving, navigate, onSave, onDelete }: DocumentToolbarProps) {
  return (
    <div className="toolbar">
      <div className="toolbar-title">
        <strong>{doc.filePath}</strong>
        <span>{route.mode === 'edit' ? 'Editing Markdown' : 'Preview'}</span>
      </div>
      <div className="toolbar-actions">
        <Button onClick={() => navigate('/')}>Files</Button>
        <Button href={doc.rawPath}>Raw</Button>
        {route.mode === 'edit' && (
          <>
            <Button onClick={() => navigate('/view' + doc.filePath)}>Preview</Button>
            <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={onSave}>Save</Button>
            <Button danger icon={<DeleteOutlined />} onClick={onDelete}>Delete</Button>
          </>
        )}
        {!contentMode && route.mode !== 'edit' && (
          <Button icon={<EditOutlined />} onClick={() => navigate('/edit' + doc.filePath)}>Edit</Button>
        )}
      </div>
    </div>
  );
}
