import { getUserNFTs } from './index';

const main = async () => {
    await getUserNFTs();
};

main().catch(console.error); 