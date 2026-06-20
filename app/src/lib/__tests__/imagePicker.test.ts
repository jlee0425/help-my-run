jest.mock('expo-image-picker', () => ({
  requestMediaLibraryPermissionsAsync: jest.fn(),
  requestCameraPermissionsAsync: jest.fn(),
  launchImageLibraryAsync: jest.fn(),
  launchCameraAsync: jest.fn(),
}));

import * as ImagePicker from 'expo-image-picker';
import { pickFromLibrary, takePhoto, toUploadFile } from '../imagePicker';

const mockReqLib = ImagePicker.requestMediaLibraryPermissionsAsync as jest.Mock;
const mockReqCam = ImagePicker.requestCameraPermissionsAsync as jest.Mock;
const mockLaunchLib = ImagePicker.launchImageLibraryAsync as jest.Mock;
const mockLaunchCam = ImagePicker.launchCameraAsync as jest.Mock;

afterEach(() => {
  jest.clearAllMocks();
});

describe('pickFromLibrary', () => {
  it('returns the first asset when permission granted and not cancelled', async () => {
    mockReqLib.mockResolvedValue({ granted: true });
    mockLaunchLib.mockResolvedValue({
      canceled: false,
      assets: [{ uri: 'file:///c.jpg', mimeType: 'image/jpeg', fileName: 'c.jpg', width: 1, height: 1 }],
    });
    const asset = await pickFromLibrary();
    expect(mockLaunchLib).toHaveBeenCalledWith({ mediaTypes: 'images', quality: 0.8, allowsEditing: false });
    expect(asset?.uri).toBe('file:///c.jpg');
  });

  it('returns null when permission denied', async () => {
    mockReqLib.mockResolvedValue({ granted: false });
    const asset = await pickFromLibrary();
    expect(asset).toBeNull();
    expect(mockLaunchLib).not.toHaveBeenCalled();
  });

  it('returns null when the picker is cancelled', async () => {
    mockReqLib.mockResolvedValue({ granted: true });
    mockLaunchLib.mockResolvedValue({ canceled: true, assets: null });
    const asset = await pickFromLibrary();
    expect(asset).toBeNull();
  });
});

describe('takePhoto', () => {
  it('returns the first asset from the camera', async () => {
    mockReqCam.mockResolvedValue({ granted: true });
    mockLaunchCam.mockResolvedValue({
      canceled: false,
      assets: [{ uri: 'file:///cam.jpg', mimeType: 'image/jpeg', fileName: 'cam.jpg', width: 1, height: 1 }],
    });
    const asset = await takePhoto();
    expect(mockLaunchCam).toHaveBeenCalledWith({ mediaTypes: 'images', quality: 0.8 });
    expect(asset?.uri).toBe('file:///cam.jpg');
  });

  it('returns null when camera permission denied', async () => {
    mockReqCam.mockResolvedValue({ granted: false });
    const asset = await takePhoto();
    expect(asset).toBeNull();
    expect(mockLaunchCam).not.toHaveBeenCalled();
  });
});

describe('toUploadFile', () => {
  it('derives uri/name/type from a full asset', () => {
    const f = toUploadFile({ uri: 'file:///c.png', mimeType: 'image/png', fileName: 'c.png' } as any);
    expect(f).toEqual({ uri: 'file:///c.png', name: 'c.png', type: 'image/png' });
  });

  it('falls back to jpeg type and a derived name when fields are missing', () => {
    const f = toUploadFile({ uri: 'file:///x', mimeType: undefined, fileName: null } as any);
    expect(f).toEqual({ uri: 'file:///x', name: 'crossfit.jpeg', type: 'image/jpeg' });
  });
});
