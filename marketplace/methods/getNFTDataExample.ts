import { getNFTData } from './index';

const main = async () => {
    const tokenId = "1"; // Replace with a valid token ID
    const nftData = await getNFTData(tokenId);
    console.log("NFT Data:", nftData);
};

main().catch(console.error); 