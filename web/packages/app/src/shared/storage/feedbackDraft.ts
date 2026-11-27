export type FeedbackDraft = {
  description: string;
  images: File[];
  archives: File[];
  requestID?: string;
  awaitingStatus?: boolean;
};

const storeName = "drafts";
let database: Promise<IDBDatabase> | undefined;

function openDatabase(): Promise<IDBDatabase> {
  database ??= new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open("csgclaw-feedback", 1);
    request.onupgradeneeded = () => request.result.createObjectStore(storeName);
    request.onsuccess = () => {
      request.result.onversionchange = () => {
        request.result.close();
        database = undefined;
      };
      resolve(request.result);
    };
    request.onerror = () => reject(request.error);
  }).catch((error: unknown) => {
    database = undefined;
    throw error;
  });
  return database;
}

export async function readFeedbackDraft(scope: string): Promise<FeedbackDraft | undefined> {
  const db = await openDatabase();
  return new Promise((resolve, reject) => {
    const transaction = db.transaction(storeName);
    const store = transaction.objectStore(storeName);
    const metadata = store.get([scope, "metadata"]);
    const attachments = store.get([scope, "attachments"]);
    transaction.oncomplete = () => {
      const draft = metadata.result as Omit<FeedbackDraft, "images" | "archives"> | undefined;
      const files = attachments.result as Pick<FeedbackDraft, "images" | "archives"> | undefined;
      resolve(draft ? { images: [], archives: [], ...files, ...draft } : undefined);
    };
    transaction.onabort = () => reject(transaction.error);
  });
}

export async function saveFeedbackDraft(
  scope: string,
  draft: FeedbackDraft,
  attachmentsChanged: boolean,
): Promise<void> {
  const db = await openDatabase();
  return new Promise((resolve, reject) => {
    const transaction = db.transaction(storeName, "readwrite");
    transaction.oncomplete = () => resolve();
    transaction.onabort = () => reject(transaction.error);
    const store = transaction.objectStore(storeName);
    const { images, archives, ...metadata } = draft;
    if (metadata.description || images.length || archives.length || metadata.requestID) {
      store.put(metadata, [scope, "metadata"]);
      if (attachmentsChanged) store.put({ images, archives }, [scope, "attachments"]);
    } else {
      store.delete([scope, "metadata"]);
      store.delete([scope, "attachments"]);
    }
  });
}
