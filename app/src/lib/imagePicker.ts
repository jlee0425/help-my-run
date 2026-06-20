import * as ImagePicker from 'expo-image-picker';

export type UploadFile = { uri: string; name: string; type: string };

const PICK_OPTIONS = { mediaTypes: 'images' as const, quality: 0.8, allowsEditing: false };

export async function pickFromLibrary(): Promise<ImagePicker.ImagePickerAsset | null> {
  const perm = await ImagePicker.requestMediaLibraryPermissionsAsync();
  if (!perm.granted) return null;
  const result = await ImagePicker.launchImageLibraryAsync(PICK_OPTIONS);
  if (result.canceled || !result.assets?.length) return null;
  return result.assets[0];
}

export async function takePhoto(): Promise<ImagePicker.ImagePickerAsset | null> {
  const perm = await ImagePicker.requestCameraPermissionsAsync();
  if (!perm.granted) return null;
  const result = await ImagePicker.launchCameraAsync({ mediaTypes: 'images', quality: 0.8 });
  if (result.canceled || !result.assets?.length) return null;
  return result.assets[0];
}

export function toUploadFile(asset: ImagePicker.ImagePickerAsset): UploadFile {
  const type = asset.mimeType ?? 'image/jpeg';
  const name = asset.fileName ?? `crossfit.${type.split('/')[1] ?? 'jpg'}`;
  return { uri: asset.uri, name, type };
}
