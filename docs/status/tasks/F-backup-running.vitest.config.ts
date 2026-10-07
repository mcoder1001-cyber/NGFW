import base from '../../../apps/web/vite.config';
import { fileURLToPath } from 'node:url';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../../apps/web/package.json',import.meta.url));
export default () => {
 const options = base({mode:'test',command:'serve'});
 return {...options, resolve:{alias:[{find:/^react\/jsx-dev-runtime$/,replacement:require.resolve('react/jsx-dev-runtime')},{find:/^@tanstack\/react-query$/,replacement:require.resolve('@tanstack/react-query')},{find:/^@testing-library\/react$/,replacement:require.resolve('@testing-library/react')},{find:/^@ngfw\/ui-kit$/,replacement:fileURLToPath(new URL('../../../packages/ui-kit/dist/index.js',import.meta.url))}]}, root:fileURLToPath(new URL('../../../apps/web',import.meta.url)), test:{...options.test, include:[fileURLToPath(new URL('./F-backup-running.review.test.tsx',import.meta.url)),'src/domains/system/backup-restore/transport.test.ts','src/domains/system/backup-restore/workflows.test.tsx']}};
};
