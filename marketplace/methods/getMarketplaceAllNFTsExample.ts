import { getMarketplaceAllNFTs } from './index';

const main = async () => {
    const allNFTs = await getMarketplaceAllNFTs();
    console.log("All NFTs in the marketplace:", allNFTs);
};

main().catch(console.error); 